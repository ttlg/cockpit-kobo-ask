package main

import (
	"slices"
	"strconv"
	"strings"
)

type draft struct {
	choices [][]int
	inputs  []string
	whole   string
}

func newDraft(a ask) *draft {
	return &draft{choices: make([][]int, len(a.Questions)), inputs: make([]string, len(a.Questions))}
}

func requiresSubmit(a ask) bool {
	if len(a.Questions) > 1 {
		return true
	}
	return slices.ContainsFunc(a.Questions, func(q askQuestion) bool {
		return q.Multiple || (q.AllowInput && len(q.Choices) > 0)
	})
}

func isTapToSend(a ask) bool {
	return !requiresSubmit(a) && len(a.Questions) == 1 && len(a.Questions[0].Choices) > 0
}

func hasSubmitButton(a ask) bool {
	return requiresSubmit(a) || (len(a.Questions) == 1 && len(a.Questions[0].Choices) == 0)
}

func isAnswered(args questionDraftArgs) bool {
	if len(args.draft.choices[args.index]) > 0 {
		return true
	}
	return args.question.AllowInput && strings.TrimSpace(args.draft.inputs[args.index]) != ""
}

func hasContent(args questionDraftArgs) bool {
	return len(args.draft.choices[args.index]) > 0 || strings.TrimSpace(args.draft.inputs[args.index]) != ""
}

type questionDraftArgs struct {
	question askQuestion
	draft    *draft
	index    int
}

type submission struct {
	answered       int
	inputs         int
	total          int
	hasWholeAnswer bool
	ready          bool
}

func describeSubmission(args submissionArgs) submission {
	result := submission{total: len(args.ask.Questions)}
	everyStartedReady := true
	for index, question := range args.ask.Questions {
		state := questionDraftArgs{question: question, draft: args.draft, index: index}
		switch {
		case isAnswered(state):
			result.answered++
			if strings.TrimSpace(args.draft.inputs[index]) != "" {
				result.inputs++
			}
		case hasContent(state):
			everyStartedReady = false
		}
	}
	result.hasWholeAnswer = len(args.ask.Questions) > 1 && strings.TrimSpace(args.draft.whole) != ""
	result.ready = everyStartedReady && (result.hasWholeAnswer || result.answered == result.total)
	return result
}

type submissionArgs struct {
	ask   ask
	draft *draft
}

func isSubmitReady(args submissionArgs) bool {
	if requiresSubmit(args.ask) {
		return describeSubmission(args).ready
	}
	return strings.TrimSpace(args.draft.inputs[0]) != ""
}

func submissionChips(args chipArgs) []string {
	chips := []string{}
	if args.summary.total > 1 {
		if args.summary.answered > 0 {
			chips = append(chips, args.tr.text("chip_answers", "answered", strconv.Itoa(args.summary.answered), "total", strconv.Itoa(args.summary.total)))
		}
		if args.summary.inputs > 0 {
			chips = append(chips, args.tr.text("chip_free_text_count", "count", strconv.Itoa(args.summary.inputs)))
		}
		if args.summary.hasWholeAnswer {
			chips = append(chips, args.tr.text("whole_title"))
		}
		return chips
	}
	if len(args.draft.choices[0]) > 0 {
		chips = append(chips, args.tr.text("chip_selection"))
	}
	if strings.TrimSpace(args.draft.inputs[0]) != "" {
		chips = append(chips, args.tr.text("chip_free_text"))
	}
	return chips
}

type chipArgs struct {
	summary submission
	draft   *draft
	tr      translator
}

func toggleChoice(args toggleArgs) {
	question := args.ask.Questions[args.questionIndex]
	selected := args.draft.choices[args.questionIndex]
	position := slices.Index(selected, args.choiceIndex)
	switch {
	case question.Multiple && position >= 0:
		args.draft.choices[args.questionIndex] = slices.Delete(slices.Clone(selected), position, position+1)
	case question.Multiple:
		args.draft.choices[args.questionIndex] = append(slices.Clone(selected), args.choiceIndex)
	case position >= 0:
		args.draft.choices[args.questionIndex] = []int{}
	default:
		args.draft.choices[args.questionIndex] = []int{args.choiceIndex}
	}
}

type toggleArgs struct {
	ask           ask
	draft         *draft
	questionIndex int
	choiceIndex   int
}

func buildAnswerRequest(args submissionArgs) answerRequest {
	single := len(args.ask.Questions) == 1
	entries := []answerEntry{}
	for index, question := range args.ask.Questions {
		if !isAnswered(questionDraftArgs{question: question, draft: args.draft, index: index}) {
			continue
		}
		var questionID *string
		if !single {
			id := question.ID
			questionID = &id
		}
		input := ""
		if question.AllowInput {
			input = strings.TrimSpace(args.draft.inputs[index])
		}
		entries = append(entries, answerEntry{QuestionID: questionID, ChoiceIndexes: append([]int{}, args.draft.choices[index]...), Input: input})
	}
	whole := ""
	if !single {
		whole = strings.TrimSpace(args.draft.whole)
	}
	return answerRequest{Answers: entries, WholeAnswer: whole}
}

func headingOf(args headingArgs) string {
	title := args.tr.text("confirmation_request")
	if args.ask.Title != nil && *args.ask.Title != "" {
		title = *args.ask.Title
	}
	if args.ask.DirectoryLabel != nil && *args.ask.DirectoryLabel != "" {
		return title + " (" + *args.ask.DirectoryLabel + ")"
	}
	return title
}

type headingArgs struct {
	ask ask
	tr  translator
}
