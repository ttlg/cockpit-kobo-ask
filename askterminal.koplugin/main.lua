local Event = require("ui/event")
local InfoMessage = require("ui/widget/infomessage")
local InputDialog = require("ui/widget/inputdialog")
local NetworkMgr = require("ui/network/manager")
local UIManager = require("ui/uimanager")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local http = require("socket.http")
local ltn12 = require("ltn12")
local logger = require("logger")
local rapidjson = require("rapidjson")
local socket = require("socket")
local socketutil = require("socketutil")
local util = require("util")

local AskScreen = require("askterminal_screen")

local SETTINGS_KEY = "askterminal"
local INPUT_PREVIEW_LENGTH = 24

local function trim(text)
    return (text or ""):match("^%s*(.-)%s*$")
end

local function preview(text)
    local single_line = trim(text):gsub("%s+", " ")
    local characters = util.splitToChars(single_line)
    if #characters <= INPUT_PREVIEW_LENGTH then
        return single_line
    end
    return table.concat(characters, "", 1, INPUT_PREVIEW_LENGTH) .. "…"
end

local function contains(list, value)
    for _, item in ipairs(list) do
        if item == value then
            return true
        end
    end
    return false
end

local AskTerminal = WidgetContainer:extend{
    name = "askterminal",
    is_doc_only = false,
}

function AskTerminal:init()
    self.settings = G_reader_settings:readSetting(SETTINGS_KEY) or {}
    self.defaults = self:loadDefaults()
    self.asks = {}
    self.current_id = nil
    self.drafts = {}
    self.hidden_ids = {}
    self.viewer = nil
    self.busy = false
    self.poll_task = function() self:poll() end
    self.ui.menu:registerToMainMenu(self)
    if self:setting("enabled") then
        UIManager:scheduleIn(3, self.poll_task)
    end
end

function AskTerminal:loadDefaults()
    local ok, defaults = pcall(dofile, self.path .. "/config.lua")
    if ok and type(defaults) == "table" then
        return defaults
    end
    return {}
end

function AskTerminal:setting(key)
    if self.settings[key] ~= nil then
        return self.settings[key]
    end
    return self.defaults[key]
end

function AskTerminal:saveSetting(key, value)
    self.settings[key] = value
    G_reader_settings:saveSetting(SETTINGS_KEY, self.settings)
end

function AskTerminal:interval()
    return tonumber(self:setting("interval")) or 15
end

function AskTerminal:addToMainMenu(menu_items)
    menu_items.askterminal = {
        text = "Cockpit Ask",
        sorting_hint = "tools",
        sub_item_table = {
            {
                text = "Ask を受け取る",
                checked_func = function() return self:setting("enabled") == true end,
                callback = function() self:setEnabled(not self:setting("enabled")) end,
            },
            {
                text = "受信箱を開く",
                callback = function() self:poll({ manual = true }) end,
            },
            {
                text = "Kobo のホームに戻る",
                callback = function() self:exitToKoboHome() end,
            },
            {
                text = "サーバー URL を変更",
                keep_menu_open = true,
                callback = function() self:editSetting({ key = "url", title = "中継サーバーの URL" }) end,
            },
            {
                text = "トークンを変更",
                keep_menu_open = true,
                callback = function() self:editSetting({ key = "token", title = "中継サーバーのトークン" }) end,
            },
        },
    }
end

function AskTerminal:editSetting(args)
    local dialog
    dialog = InputDialog:new{
        title = args.title,
        input = self:setting(args.key) or "",
        buttons = {{
            {
                text = "キャンセル",
                id = "close",
                callback = function() UIManager:close(dialog) end,
            },
            {
                text = "保存",
                is_enter_default = true,
                callback = function()
                    self:saveSetting(args.key, dialog:getInputText())
                    UIManager:close(dialog)
                end,
            },
        }},
    }
    UIManager:show(dialog)
    dialog:onShowKeyboard()
end

function AskTerminal:setEnabled(enabled)
    self:saveSetting("enabled", enabled)
    UIManager:unschedule(self.poll_task)
    if enabled then
        self:poll({ manual = true })
        return
    end
    self:closeViewer()
    UIManager:show(InfoMessage:new{ text = "Ask の受け取りを止めました。", timeout = 2 })
end

function AskTerminal:request(args)
    local url = self:setting("url")
    local token = self:setting("token")
    if not url or url == "" or not token or token == "" then
        return nil, "サーバー URL とトークンを設定してください。"
    end
    local chunks = {}
    local headers = {
        ["Authorization"] = "Bearer " .. token,
        ["Accept"] = "application/json",
    }
    local body = args.body or ""
    headers["Content-Type"] = "application/json"
    headers["Content-Length"] = tostring(#body)
    socketutil:set_timeout(socketutil.LARGE_BLOCK_TIMEOUT, socketutil.LARGE_TOTAL_TIMEOUT)
    local code = socket.skip(1, http.request{
        url = url .. args.path,
        method = args.method,
        headers = headers,
        source = ltn12.source.string(body),
        sink = ltn12.sink.table(chunks),
    })
    socketutil:reset_timeout()
    local ok, decoded = pcall(rapidjson.decode, table.concat(chunks))
    if code == 200 and ok and type(decoded) == "table" then
        return decoded
    end
    logger.warn("askterminal: request failed", args.method, args.path, code, table.concat(chunks))
    if code == 401 then
        return nil, "トークンが違います。"
    end
    return nil, "中継サーバーにつながりません（" .. tostring(code) .. "）。"
end

function AskTerminal:scheduleNext(seconds)
    UIManager:unschedule(self.poll_task)
    if self:setting("enabled") then
        UIManager:scheduleIn(seconds or self:interval(), self.poll_task)
    end
end

function AskTerminal:indexOf(id)
    for index, ask in ipairs(self.asks) do
        if ask.id == id then
            return index
        end
    end
    return nil
end

function AskTerminal:idsKey(asks)
    local ids = {}
    for _, ask in ipairs(asks) do
        table.insert(ids, ask.id)
    end
    return table.concat(ids, ",")
end

function AskTerminal:pruneState()
    local open = {}
    for _, ask in ipairs(self.asks) do
        open[ask.id] = true
    end
    for id in pairs(self.drafts) do
        if not open[id] then
            self.drafts[id] = nil
        end
    end
    for id in pairs(self.hidden_ids) do
        if not open[id] then
            self.hidden_ids[id] = nil
        end
    end
end

function AskTerminal:firstUnhiddenId()
    for _, ask in ipairs(self.asks) do
        if not self.hidden_ids[ask.id] then
            return ask.id
        end
    end
    return nil
end

function AskTerminal:showEmptyState()
    UIManager:show(InfoMessage:new{
        text = "未読はありません\n\nタスクからの質問とお知らせがここに表示されます。",
        timeout = 3,
    })
end

function AskTerminal:poll(options)
    local manual = options and options.manual
    if self.busy then
        return self:scheduleNext()
    end
    if not NetworkMgr:isConnected() then
        if manual then
            UIManager:show(InfoMessage:new{ text = "Wi-Fi に接続してください。", timeout = 3 })
        end
        return self:scheduleNext()
    end
    local response, err = self:request({ method = "GET", path = "/asks" })
    if not response then
        if manual then
            UIManager:show(InfoMessage:new{ text = err, timeout = 4 })
        end
        return self:scheduleNext()
    end
    local previous_key = self:idsKey(self.asks)
    self.asks = response.asks or {}
    self:pruneState()
    self:updateView({ manual = manual, changed = previous_key ~= self:idsKey(self.asks) })
    self:scheduleNext()
end

function AskTerminal:updateView(args)
    if #self.asks == 0 then
        self:closeViewer()
        if args.manual then
            self:showEmptyState()
        end
        return
    end
    if self.viewer then
        if args.changed then
            self:render()
        end
        return
    end
    if args.manual then
        self.current_id = self.current_id and self:indexOf(self.current_id) and self.current_id or self.asks[1].id
        return self:render()
    end
    local unhidden_id = self:firstUnhiddenId()
    if unhidden_id then
        self.current_id = unhidden_id
        self:render()
    end
end

function AskTerminal:currentAsk()
    local index = self:indexOf(self.current_id)
    if not index then
        index = 1
        self.current_id = self.asks[1].id
    end
    return self.asks[index], index
end

function AskTerminal:draftOf(ask)
    if not self.drafts[ask.id] then
        local questions = {}
        for question_index in ipairs(ask.questions) do
            questions[question_index] = { choices = {}, input = "" }
        end
        self.drafts[ask.id] = { questions = questions, whole_answer = "" }
    end
    return self.drafts[ask.id]
end

function AskTerminal:requiresSubmit(ask)
    if #ask.questions > 1 then
        return true
    end
    for _, question in ipairs(ask.questions) do
        if question.multiple or (question.allowInput and #question.choices > 0) then
            return true
        end
    end
    return false
end

function AskTerminal:isTapToSend(ask)
    return not self:requiresSubmit(ask) and #ask.questions[1].choices > 0
end

function AskTerminal:isAnswered(args)
    if #args.draft.choices > 0 then
        return true
    end
    return args.question.allowInput and trim(args.draft.input) ~= ""
end

function AskTerminal:hasContent(draft)
    return #draft.choices > 0 or trim(draft.input) ~= ""
end

function AskTerminal:submission(ask)
    local draft = self:draftOf(ask)
    local answered = 0
    local inputs = 0
    local every_started_ready = true
    for question_index, question in ipairs(ask.questions) do
        local question_draft = draft.questions[question_index]
        local is_answered = self:isAnswered({ question = question, draft = question_draft })
        if is_answered then
            answered = answered + 1
            if trim(question_draft.input) ~= "" then
                inputs = inputs + 1
            end
        elseif self:hasContent(question_draft) then
            every_started_ready = false
        end
    end
    local has_whole_answer = #ask.questions > 1 and trim(draft.whole_answer) ~= ""
    local total = #ask.questions
    return {
        answered = answered,
        inputs = inputs,
        total = total,
        has_whole_answer = has_whole_answer,
        ready = every_started_ready and (has_whole_answer or answered == total),
        blocked = not every_started_ready,
    }
end

function AskTerminal:submissionChips(args)
    local chips = {}
    if args.total > 1 then
        if args.answered > 0 then
            table.insert(chips, string.format("回答 %d/%d", args.answered, args.total))
        end
        if args.inputs > 0 then
            table.insert(chips, string.format("自由記入 %d", args.inputs))
        end
        if args.has_whole_answer then
            table.insert(chips, "Ask 全体に回答")
        end
        return chips
    end
    local question_draft = args.draft.questions[1]
    if #question_draft.choices > 0 then
        table.insert(chips, "選択")
    end
    if trim(question_draft.input) ~= "" then
        table.insert(chips, "自由記入")
    end
    return chips
end

function AskTerminal:headerBlock(args)
    local ask = args.ask
    local block = {
        { kind = "text", style = "meta", text = string.format("%d/%d · %s", args.index, #self.asks, ask.time) },
        { kind = "text", style = "summary", text = ask.summary },
    }
    if ask.mediaCount > 0 then
        table.insert(block, {
            kind = "text",
            style = "note",
            text = string.format("画像や動画が %d 件添付されています。Kobo では表示できないため、Mac で確認してください。", ask.mediaCount),
        })
    end
    return block
end

function AskTerminal:questionHeader(args)
    local question = args.question
    local header = question.title
    if question.multiple and #question.choices > 0 then
        local badge = #args.draft.choices > 0 and string.format("［%d件選択中］", #args.draft.choices) or "［複数選択］"
        header = header ~= "" and (header .. "  " .. badge) or badge
    end
    return header
end

function AskTerminal:questionBlock(args)
    local ask = args.ask
    local question = ask.questions[args.question_index]
    local question_draft = self:draftOf(ask).questions[args.question_index]
    local block = {}
    if #ask.questions > 1 then
        table.insert(block, { kind = "separator" })
    end
    local header = self:questionHeader({ question = question, draft = question_draft })
    if header ~= "" then
        table.insert(block, { kind = "text", style = "heading", text = header })
    end
    for choice_index, choice in ipairs(question.choices) do
        table.insert(block, {
            kind = "button",
            text = self:choiceLabel({ ask = ask, question_index = args.question_index, choice_index = choice_index, choice = choice }),
            description = question.choiceDescriptions[choice_index],
            callback = function() self:onChoice({ ask = ask, question_index = args.question_index, choice_index = choice_index }) end,
        })
    end
    if question.allowInput then
        table.insert(block, {
            kind = "button",
            text = self:inputLabel({ input = question_draft.input, placeholder = "自由入力で返答" }),
            callback = function() self:editAnswerText({ ask = ask, question_index = args.question_index }) end,
        })
    end
    return block
end

function AskTerminal:wholeAnswerBlock(ask)
    return {
        { kind = "separator" },
        { kind = "text", style = "heading", text = "Ask 全体に回答" },
        { kind = "text", style = "note", text = "Ask 全体に宛てた回答を書けます。上の各質問への回答と一緒に送信され、空欄の質問は未回答のまま届きます。" },
        {
            kind = "button",
            text = self:inputLabel({ input = self:draftOf(ask).whole_answer, placeholder = "Ask 全体への回答..." }),
            callback = function() self:editAnswerText({ ask = ask }) end,
        },
    }
end

function AskTerminal:submissionBlock(ask)
    if not self:requiresSubmit(ask) then
        return nil
    end
    local submission = self:submission(ask)
    if submission.ready then
        local chips = self:submissionChips({
            total = submission.total,
            answered = submission.answered,
            inputs = submission.inputs,
            has_whole_answer = submission.has_whole_answer,
            draft = self:draftOf(ask),
        })
        return {{ kind = "text", style = "note", text = "送信する内容：" .. table.concat(chips, " ・ ") }}
    end
    if #ask.questions > 1 then
        return {{
            kind = "text",
            style = "note",
            text = submission.blocked
                and "添付がある質問には選択が必要です。選択肢を選ぶか、添付を削除してください。"
                or "すべての質問に答えるか、Ask 全体への回答を書いてください。",
        }}
    end
    return nil
end

function AskTerminal:blocks(args)
    local ask = args.ask
    local blocks = { self:headerBlock(args) }
    for question_index in ipairs(ask.questions) do
        table.insert(blocks, self:questionBlock({ ask = ask, question_index = question_index }))
    end
    if #ask.questions > 1 then
        table.insert(blocks, self:wholeAnswerBlock(ask))
    end
    local submission_block = self:submissionBlock(ask)
    if submission_block then
        table.insert(blocks, submission_block)
    end
    return blocks
end

function AskTerminal:choiceLabel(args)
    if self:isTapToSend(args.ask) then
        return args.choice
    end
    local question = args.ask.questions[args.question_index]
    local is_selected = contains(self:draftOf(args.ask).questions[args.question_index].choices, args.choice_index)
    local mark
    if question.multiple then
        mark = is_selected and "☑ " or "☐ "
    else
        mark = is_selected and "● " or "○ "
    end
    return mark .. args.choice
end

function AskTerminal:inputLabel(args)
    local text = trim(args.input)
    if text == "" then
        return "✎ " .. args.placeholder
    end
    return "✎ " .. preview(text)
end

function AskTerminal:hasSubmitButton(ask)
    if self:requiresSubmit(ask) then
        return true
    end
    return #ask.questions[1].choices == 0
end

function AskTerminal:isSubmitReady(ask)
    if self:requiresSubmit(ask) then
        return self:submission(ask).ready
    end
    return trim(self:draftOf(ask).questions[1].input) ~= ""
end

function AskTerminal:footerRows(args)
    local ask = args.ask
    local count = #self.asks
    local rows = {}
    if self:hasSubmitButton(ask) then
        table.insert(rows, {{
            text = "送信",
            enabled = self:isSubmitReady(ask),
            callback = function() self:submit(ask) end,
        }})
    end
    table.insert(rows, {
        {
            text = "閉じる",
            callback = function() self:closeAsk(ask) end,
        },
        {
            text = "◀ 前へ",
            enabled = args.index > 1,
            callback = function() self:move(-1) end,
        },
        {
            text = count > 1 and string.format("%d 件が未処理です", count) or "1 件が未処理です",
            enabled = false,
            callback = function() end,
        },
        {
            text = "次へ ▶",
            enabled = args.index < count,
            callback = function() self:move(1) end,
        },
    })
    table.insert(rows, {{
        text = "Kobo のホームに戻る",
        callback = function() self:exitToKoboHome() end,
    }})
    return rows
end

function AskTerminal:exitToKoboHome()
    self:closeViewer()
    UIManager:broadcastEvent(Event:new("Exit"))
end

function AskTerminal:render()
    local ask, index = self:currentAsk()
    local previous = self.viewer
    local scroll_offset = previous and previous.ask_id == ask.id and previous:getScrollOffset() or nil
    self.viewer = AskScreen:new{
        title = ask.heading,
        blocks = self:blocks({ ask = ask, index = index }),
        footer_rows = self:footerRows({ ask = ask, index = index }),
        scroll_offset = scroll_offset,
        close_callback = function() self:onViewerClosedByUser() end,
    }
    self.viewer.ask_id = ask.id
    UIManager:show(self.viewer, previous and "ui" or "full")
    if previous then
        UIManager:close(previous)
    end
end

function AskTerminal:onViewerClosedByUser()
    self.viewer = nil
    for _, ask in ipairs(self.asks) do
        self.hidden_ids[ask.id] = true
    end
end

function AskTerminal:closeViewer()
    if not self.viewer then
        return
    end
    local viewer = self.viewer
    self.viewer = nil
    UIManager:close(viewer, "full")
end

function AskTerminal:move(step)
    local _, index = self:currentAsk()
    local target = self.asks[index + step]
    if target then
        self.current_id = target.id
        self:render()
    end
end

function AskTerminal:onChoice(args)
    local ask = args.ask
    local question = ask.questions[args.question_index]
    local question_draft = self:draftOf(ask).questions[args.question_index]
    if self:isTapToSend(ask) then
        question_draft.choices = { args.choice_index }
        return self:submit(ask)
    end
    if question.multiple then
        if contains(question_draft.choices, args.choice_index) then
            local remaining = {}
            for _, choice_index in ipairs(question_draft.choices) do
                if choice_index ~= args.choice_index then
                    table.insert(remaining, choice_index)
                end
            end
            question_draft.choices = remaining
        else
            table.insert(question_draft.choices, args.choice_index)
        end
    elseif question_draft.choices[1] == args.choice_index then
        question_draft.choices = {}
    else
        question_draft.choices = { args.choice_index }
    end
    self:render()
end

function AskTerminal:editAnswerText(args)
    local ask = args.ask
    local draft = self:draftOf(ask)
    local question = args.question_index and ask.questions[args.question_index]
    local current = question and draft.questions[args.question_index].input or draft.whole_answer
    local description
    if question then
        description = question.title ~= "" and question.title or nil
    else
        description = "Ask 全体に宛てた回答を書けます。上の各質問への回答と一緒に送信され、空欄の質問は未回答のまま届きます。"
    end
    local dialog
    dialog = InputDialog:new{
        title = question and "自由入力で返答" or "Ask 全体に回答",
        description = description,
        input = current,
        input_hint = question and "自由入力で返答" or "Ask 全体への回答...",
        allow_newline = true,
        buttons = {{
            {
                text = "キャンセル",
                id = "close",
                callback = function() UIManager:close(dialog) end,
            },
            {
                text = "決定",
                callback = function()
                    local text = dialog:getInputText()
                    if question then
                        draft.questions[args.question_index].input = text
                    else
                        draft.whole_answer = text
                    end
                    UIManager:close(dialog)
                    self:render()
                end,
            },
        }},
    }
    UIManager:show(dialog)
    dialog:onShowKeyboard()
end

function AskTerminal:answerEntries(ask)
    local draft = self:draftOf(ask)
    local single = #ask.questions == 1
    local entries = {}
    for question_index, question in ipairs(ask.questions) do
        local question_draft = draft.questions[question_index]
        if self:isAnswered({ question = question, draft = question_draft }) then
            table.insert(entries, {
                questionId = single and rapidjson.null or question.id,
                choiceIndexes = rapidjson.array(question_draft.choices),
                input = question.allowInput and trim(question_draft.input) or "",
            })
        end
    end
    return entries
end

function AskTerminal:afterResolved(ask)
    local index = self:indexOf(ask.id)
    if index then
        table.remove(self.asks, index)
    end
    self:pruneState()
    if #self.asks == 0 then
        self:closeViewer()
    else
        self.current_id = self.asks[1].id
        self:render()
    end
    self:scheduleNext(3)
end

function AskTerminal:send(args)
    self.busy = true
    local response, err = self:request({ method = "POST", path = "/asks/" .. args.ask.id .. "/" .. args.action, body = args.body })
    self.busy = false
    if not response then
        UIManager:show(InfoMessage:new{ text = args.failure .. "\n" .. err, timeout = 4 })
        return
    end
    self:afterResolved(args.ask)
end

function AskTerminal:submit(ask)
    local draft = self:draftOf(ask)
    local body = rapidjson.encode({
        answers = rapidjson.array(self:answerEntries(ask)),
        wholeAnswer = #ask.questions > 1 and trim(draft.whole_answer) or "",
    })
    self:send({ ask = ask, action = "answer", body = body, failure = "送信できませんでした。" })
end

function AskTerminal:closeAsk(ask)
    self:send({ ask = ask, action = "close", body = "{}", failure = "閉じられませんでした。" })
end

function AskTerminal:onCloseWidget()
    UIManager:unschedule(self.poll_task)
    self:closeViewer()
end

function AskTerminal:onSuspend()
    UIManager:unschedule(self.poll_task)
end

function AskTerminal:onResume()
    if self:setting("enabled") then
        self:scheduleNext(5)
    end
end

return AskTerminal
