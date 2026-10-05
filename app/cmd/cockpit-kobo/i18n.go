package main

import "strings"

var translations = map[string]map[string]string{
	"en": {
		"empty_title":          "Nothing to read",
		"empty_description":    "Questions and notices from your tasks appear here.",
		"confirmation_request": "Confirmation request",
		"media_note":           "This ask has {{count}} image or video attachments. The Kobo cannot show them, so check them on your Mac.",
		"badge_multiple":       "Multiple choice",
		"badge_selected":       "{{count}} selected",
		"input_placeholder":    "Reply with free text",
		"whole_title":          "Reply to the whole ask",
		"whole_description":    "Add a reply addressed to the whole ask. It is sent together with your answers above; questions you leave blank stay unanswered.",
		"whole_placeholder":    "Reply to the whole ask...",
		"send_summary":         "Will send: {{chips}}",
		"chip_separator":       " · ",
		"chip_answers":         "Answers {{answered}}/{{total}}",
		"chip_free_text_count": "Free text {{count}}",
		"chip_selection":       "Selection",
		"chip_free_text":       "Free text",
		"incomplete":           "Answer every question, or add a reply to the whole ask.",
		"send":                 "Send",
		"sending":              "Sending…",
		"close":                "Close",
		"previous":             "◀ Previous",
		"next":                 "Next ▶",
		"queue":                "{{count}} items waiting",
		"single":               "1 item waiting",
		"kobo_home":            "Return to Kobo home",
		"send_failed":          "Could not send the answer: {{error}}",
		"close_failed":         "Could not close the ask: {{error}}",
		"relay_unreachable":    "Cannot reach the relay: {{error}}",
		"wrong_token":          "The relay token is incorrect.",
		"cancel":               "Cancel",
		"done":                 "Done",
		"space":                "space",
		"usb_title":            "USB cable connected",
		"usb_description":      "To connect to your computer, return to Kobo home. The Kobo then asks whether to connect.",
		"usb_connect":          "Connect to computer",
		"usb_keep_charging":    "Keep charging",
	},
	"ja": {
		"empty_title":          "未読はありません",
		"empty_description":    "タスクからの質問とお知らせがここに表示されます。",
		"confirmation_request": "確認リクエスト",
		"media_note":           "画像や動画が {{count}} 件添付されています。Kobo では表示できないため、Mac で確認してください。",
		"badge_multiple":       "複数選択",
		"badge_selected":       "{{count}}件選択中",
		"input_placeholder":    "自由入力で返答",
		"whole_title":          "Ask 全体に回答",
		"whole_description":    "Ask 全体に宛てた回答を書けます。上の各質問への回答と一緒に送信され、空欄の質問は未回答のまま届きます。",
		"whole_placeholder":    "Ask 全体への回答...",
		"send_summary":         "送信する内容：{{chips}}",
		"chip_separator":       " ・ ",
		"chip_answers":         "回答 {{answered}}/{{total}}",
		"chip_free_text_count": "自由記入 {{count}}",
		"chip_selection":       "選択",
		"chip_free_text":       "自由記入",
		"incomplete":           "すべての質問に答えるか、Ask 全体への回答を書いてください。",
		"send":                 "送信",
		"sending":              "送信中…",
		"close":                "閉じる",
		"previous":             "◀ 前へ",
		"next":                 "次へ ▶",
		"queue":                "{{count}} 件が未処理です",
		"single":               "1 件が未処理です",
		"kobo_home":            "Kobo のホームに戻る",
		"send_failed":          "送信できませんでした：{{error}}",
		"close_failed":         "閉じられませんでした：{{error}}",
		"relay_unreachable":    "中継サーバーにつながりません：{{error}}",
		"wrong_token":          "トークンが違います。",
		"cancel":               "キャンセル",
		"done":                 "決定",
		"space":                "space",
		"usb_title":            "USB ケーブルが接続されました",
		"usb_description":      "パソコンに接続するには、Kobo のホームに戻ります。戻ると、接続するかどうかを Kobo が確認します。",
		"usb_connect":          "パソコンに接続",
		"usb_keep_charging":    "充電だけ",
	},
}

type translator struct {
	language string
}

func (t translator) text(key string, values ...string) string {
	table, ok := translations[t.language]
	if !ok {
		table = translations["en"]
	}
	result, ok := table[key]
	if !ok {
		result = translations["en"][key]
	}
	for index := 0; index+1 < len(values); index += 2 {
		result = strings.ReplaceAll(result, "{{"+values[index]+"}}", values[index+1])
	}
	return result
}
