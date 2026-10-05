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

local AskTerminal = WidgetContainer:extend{
    name = "askterminal",
    is_doc_only = false,
}

function AskTerminal:init()
    self.settings = G_reader_settings:readSetting(SETTINGS_KEY) or {}
    self.defaults = self:loadDefaults()
    self.skipped = {}
    self.viewer = nil
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
                text = "今すぐ確認",
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

function AskTerminal:scheduleNext()
    UIManager:unschedule(self.poll_task)
    if self:setting("enabled") then
        UIManager:scheduleIn(self:interval(), self.poll_task)
    end
end

function AskTerminal:poll(options)
    local manual = options and options.manual
    if self.viewer then
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
    local open = {}
    for _, ask in ipairs(response.asks or {}) do
        open[ask.id] = true
    end
    for id in pairs(self.skipped) do
        if not open[id] then
            self.skipped[id] = nil
        end
    end
    for _, ask in ipairs(response.asks or {}) do
        if not self.skipped[ask.id] or manual then
            self.skipped[ask.id] = nil
            self:startAsk(ask)
            return self:scheduleNext()
        end
    end
    if manual then
        UIManager:show(InfoMessage:new{ text = "未回答の Ask はありません。", timeout = 2 })
    end
    self:scheduleNext()
end

function AskTerminal:startAsk(ask)
    self:showQuestion({ ask = ask, index = 1, answers = {}, selected = {} })
end

function AskTerminal:questionText(state)
    local question = state.ask.questions[state.index]
    local parts = { state.ask.summary }
    if #state.ask.questions > 1 then
        table.insert(parts, "――――――――")
        table.insert(parts, string.format("質問 %d / %d", state.index, #state.ask.questions))
    end
    if question and question.summary ~= "" then
        table.insert(parts, question.summary)
    end
    if not state.ask.answerable then
        table.insert(parts, "――――――――")
        table.insert(parts, state.ask.unanswerableReason)
    end
    return table.concat(parts, "\n\n")
end

function AskTerminal:choiceButtons(state)
    local question = state.ask.questions[state.index]
    local rows = {}
    if not state.ask.answerable then
        return rows
    end
    for choice_index, choice in ipairs(question.choices) do
        local label = choice
        if question.multiple then
            label = (state.selected[choice_index] and "☑ " or "☐ ") .. choice
        end
        table.insert(rows, {{
            text = label,
            callback = function() self:onChoice({ state = state, choice_index = choice_index }) end,
        }})
    end
    if question.multiple then
        table.insert(rows, {{
            text = "決定",
            callback = function() self:confirmMultiple(state) end,
        }})
    end
    return rows
end

function AskTerminal:showQuestion(state)
    local rows = self:choiceButtons(state)
    table.insert(rows, {{
        text = "あとで",
        callback = function()
            self.skipped[state.ask.id] = true
            self:closeViewer()
        end,
    }})
    local title = state.ask.title ~= "" and state.ask.title or "Cockpit Ask"
    self:closeViewer()
    self.viewer = TextViewer:new{
        title = title,
        text = self:questionText(state),
        buttons_table = rows,
        add_default_buttons = false,
        close_callback = function() self.viewer = nil end,
    }
    UIManager:show(self.viewer, "full")
end

function AskTerminal:closeViewer()
    if not self.viewer then
        return
    end
    local viewer = self.viewer
    self.viewer = nil
    UIManager:close(viewer)
end

function AskTerminal:onChoice(args)
    local state = args.state
    local question = state.ask.questions[state.index]
    if question.multiple then
        state.selected[args.choice_index] = not state.selected[args.choice_index] or nil
        return self:showQuestion(state)
    end
    self:recordAnswer({ state = state, indexes = { args.choice_index } })
end

function AskTerminal:confirmMultiple(state)
    local indexes = {}
    for choice_index in pairs(state.selected) do
        table.insert(indexes, choice_index)
    end
    if #indexes == 0 then
        UIManager:show(InfoMessage:new{ text = "1 つ以上選んでください。", timeout = 2 })
        return
    end
    table.sort(indexes)
    self:recordAnswer({ state = state, indexes = indexes })
end

function AskTerminal:recordAnswer(args)
    local state = args.state
    local question = state.ask.questions[state.index]
    table.insert(state.answers, {
        questionId = type(question.id) == "string" and question.id or rapidjson.null,
        choiceIndexes = rapidjson.array(args.indexes),
    })
    if state.index < #state.ask.questions then
        return self:showQuestion({ ask = state.ask, index = state.index + 1, answers = state.answers, selected = {} })
    end
    self:submit(state)
end

function AskTerminal:submit(state)
    local body = rapidjson.encode({ answers = rapidjson.array(state.answers) })
    local response, err = self:request({ method = "POST", path = "/asks/" .. state.ask.id .. "/answer", body = body })
    self:closeViewer()
    if not response then
        UIManager:show(InfoMessage:new{ text = "送信できませんでした。\n" .. err, timeout = 4 })
        return self:scheduleNext()
    end
    UIManager:show(InfoMessage:new{ text = "回答を送信しました。", timeout = 2 })
    UIManager:unschedule(self.poll_task)
    UIManager:scheduleIn(3, self.poll_task)
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
        UIManager:scheduleIn(5, self.poll_task)
    end
end

return AskTerminal
