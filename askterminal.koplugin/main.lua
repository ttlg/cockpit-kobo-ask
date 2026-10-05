local InfoMessage = require("ui/widget/infomessage")
local InputDialog = require("ui/widget/inputdialog")
local NetworkMgr = require("ui/network/manager")
local TextViewer = require("ui/widget/textviewer")
local UIManager = require("ui/uimanager")
local WidgetContainer = require("ui/widget/container/widgetcontainer")
local http = require("socket.http")
local ltn12 = require("ltn12")
local logger = require("logger")
local rapidjson = require("rapidjson")
local socket = require("socket")
local socketutil = require("socketutil")

local SETTINGS_KEY = "askterminal"
local SEPARATOR = "――――――――"

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
    self.submitting = false
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
    if args.body then
        headers["Content-Type"] = "application/json"
        headers["Content-Length"] = tostring(#args.body)
    end
    socketutil:set_timeout(socketutil.LARGE_BLOCK_TIMEOUT, socketutil.LARGE_TOTAL_TIMEOUT)
    local code = socket.skip(1, http.request{
        url = url .. args.path,
        method = args.method,
        headers = headers,
        source = args.body and ltn12.source.string(args.body) or nil,
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

function AskTerminal:poll(options)
    local manual = options and options.manual
    if self.submitting then
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
            UIManager:show(InfoMessage:new{ text = "未読はありません。", timeout = 2 })
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
        self.current_id = self.asks[1].id
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
        local selected = {}
        for question_index in ipairs(ask.questions) do
            selected[question_index] = {}
        end
        self.drafts[ask.id] = { selected = selected }
    end
    return self.drafts[ask.id]
end

function AskTerminal:isSimple(ask)
    return #ask.questions == 1 and not ask.questions[1].multiple
end

function AskTerminal:isReady(ask)
    local draft = self:draftOf(ask)
    for question_index in ipairs(ask.questions) do
        if next(draft.selected[question_index]) == nil then
            return false
        end
    end
    return true
end

function AskTerminal:bodyText(args)
    local ask = args.ask
    local parts = {
        string.format("%d/%d · %s", args.index, #self.asks, ask.time),
        ask.summary,
    }
    if #ask.questions > 1 then
        for question_index, question in ipairs(ask.questions) do
            table.insert(parts, SEPARATOR)
            table.insert(parts, string.format("【%d】%s", question_index, question.summary))
        end
    elseif ask.questions[1] and ask.questions[1].summary ~= "" then
        table.insert(parts, ask.questions[1].summary)
    end
    if not ask.answerable then
        table.insert(parts, SEPARATOR)
        table.insert(parts, ask.unanswerableReason)
    end
    return table.concat(parts, "\n\n")
end

function AskTerminal:choiceLabel(args)
    if self:isSimple(args.ask) then
        return args.choice
    end
    local question = args.ask.questions[args.question_index]
    local prefix = #args.ask.questions > 1 and string.format("【%d】", args.question_index) or ""
    local selected = self:draftOf(args.ask).selected[args.question_index][args.choice_index]
    local mark
    if question.multiple then
        mark = selected and "☑ " or "☐ "
    else
        mark = selected and "● " or "○ "
    end
    return mark .. prefix .. args.choice
end

function AskTerminal:answerRows(ask)
    local rows = {}
    if not ask.answerable then
        return rows
    end
    for question_index, question in ipairs(ask.questions) do
        for choice_index, choice in ipairs(question.choices) do
            table.insert(rows, {{
                text = self:choiceLabel({ ask = ask, question_index = question_index, choice_index = choice_index, choice = choice }),
                callback = function() self:onChoice({ ask = ask, question_index = question_index, choice_index = choice_index }) end,
            }})
        end
    end
    if not self:isSimple(ask) then
        table.insert(rows, {{
            text = "送信",
            enabled = self:isReady(ask),
            callback = function() self:submit(ask) end,
        }})
    end
    return rows
end

function AskTerminal:navigationRow(index)
    local count = #self.asks
    return {
        {
            text = "◀ 前へ",
            enabled = index > 1,
            callback = function() self:move(-1) end,
        },
        {
            text = string.format("%d 件が未処理です", count),
            enabled = false,
            callback = function() end,
        },
        {
            text = "次へ ▶",
            enabled = index < count,
            callback = function() self:move(1) end,
        },
    }
end

function AskTerminal:render()
    local ask, index = self:currentAsk()
    local rows = self:answerRows(ask)
    table.insert(rows, self:navigationRow(index))
    local refresh = self.viewer and "ui" or "full"
    self:closeViewer()
    self.viewer = TextViewer:new{
        title = ask.heading,
        text = self:bodyText({ ask = ask, index = index }),
        buttons_table = rows,
        add_default_buttons = false,
        close_callback = function() self:onViewerClosedByUser() end,
    }
    UIManager:show(self.viewer, refresh)
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
    UIManager:close(viewer)
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
    local draft = self:draftOf(ask)
    if self:isSimple(ask) then
        draft.selected[1] = { [args.choice_index] = true }
        return self:submit(ask)
    end
    local selected = draft.selected[args.question_index]
    if ask.questions[args.question_index].multiple then
        selected[args.choice_index] = not selected[args.choice_index] or nil
    else
        draft.selected[args.question_index] = { [args.choice_index] = true }
    end
    self:render()
end

function AskTerminal:answerGroups(ask)
    local draft = self:draftOf(ask)
    local groups = {}
    for question_index, question in ipairs(ask.questions) do
        local indexes = {}
        for choice_index in pairs(draft.selected[question_index]) do
            table.insert(indexes, choice_index)
        end
        table.sort(indexes)
        table.insert(groups, {
            questionId = type(question.id) == "string" and question.id or rapidjson.null,
            choiceIndexes = rapidjson.array(indexes),
        })
    end
    return groups
end

function AskTerminal:submit(ask)
    local body = rapidjson.encode({ answers = rapidjson.array(self:answerGroups(ask)) })
    self.submitting = true
    local response, err = self:request({ method = "POST", path = "/asks/" .. ask.id .. "/answer", body = body })
    self.submitting = false
    if not response then
        UIManager:show(InfoMessage:new{ text = "送信できませんでした。\n" .. err, timeout = 4 })
        return
    end
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
    UIManager:show(InfoMessage:new{ text = "回答を送信しました。", timeout = 2 })
    self:scheduleNext(3)
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
