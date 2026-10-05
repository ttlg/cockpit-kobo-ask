package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type askQuestion struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Choices            []string `json:"choices"`
	ChoiceDescriptions []string `json:"choiceDescriptions"`
	Multiple           bool     `json:"multiple"`
	AllowInput         bool     `json:"allowInput"`
}

type ask struct {
	ID             string        `json:"id"`
	Title          *string       `json:"title"`
	DirectoryLabel *string       `json:"directoryLabel"`
	Time           string        `json:"time"`
	Summary        string        `json:"summary"`
	MediaCount     int           `json:"mediaCount"`
	Questions      []askQuestion `json:"questions"`
}

type answerEntry struct {
	QuestionID    *string `json:"questionId"`
	ChoiceIndexes []int   `json:"choiceIndexes"`
	Input         string  `json:"input"`
}

type answerRequest struct {
	Answers     []answerEntry `json:"answers"`
	WholeAnswer string        `json:"wholeAnswer"`
}

type relayClient struct {
	url    string
	token  string
	client *http.Client
}

type relayError struct {
	status int
}

func (e relayError) Error() string {
	return fmt.Sprintf("relay returned %d", e.status)
}

func newRelayClient(args relayClientArgs) relayClient {
	return relayClient{url: args.url, token: args.token, client: &http.Client{Timeout: 20 * time.Second}}
}

type relayClientArgs struct {
	url   string
	token string
}

func (c relayClient) do(args relayRequest) ([]byte, error) {
	request, err := http.NewRequest(args.method, c.url+args.path, bytes.NewReader(args.body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, relayError{status: response.StatusCode}
	}
	return body, nil
}

type relayRequest struct {
	method string
	path   string
	body   []byte
}

func (c relayClient) listAsks() ([]ask, error) {
	body, err := c.do(relayRequest{method: http.MethodGet, path: "/asks"})
	if err != nil {
		return nil, err
	}
	payload := struct {
		Asks []ask `json:"asks"`
	}{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Asks, nil
}

func (c relayClient) answer(args answerArgs) error {
	body, err := json.Marshal(args.request)
	if err != nil {
		return err
	}
	_, err = c.do(relayRequest{method: http.MethodPost, path: "/asks/" + args.askID + "/answer", body: body})
	return err
}

type answerArgs struct {
	askID   string
	request answerRequest
}

func (c relayClient) closeAsk(askID string) error {
	_, err := c.do(relayRequest{method: http.MethodPost, path: "/asks/" + askID + "/close", body: []byte("{}")})
	return err
}
