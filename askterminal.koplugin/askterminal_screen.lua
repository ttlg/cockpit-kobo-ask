local Blitbuffer = require("ffi/blitbuffer")
local Button = require("ui/widget/button")
local ButtonTable = require("ui/widget/buttontable")
local CenterContainer = require("ui/widget/container/centercontainer")
local Device = require("device")
local Font = require("ui/font")
local FrameContainer = require("ui/widget/container/framecontainer")
local Geom = require("ui/geometry")
local HorizontalGroup = require("ui/widget/horizontalgroup")
local HorizontalSpan = require("ui/widget/horizontalspan")
local InputContainer = require("ui/widget/container/inputcontainer")
local LineWidget = require("ui/widget/linewidget")
local ScrollableContainer = require("ui/widget/container/scrollablecontainer")
local Size = require("ui/size")
local TextBoxWidget = require("ui/widget/textboxwidget")
local TitleBar = require("ui/widget/titlebar")
local UIManager = require("ui/uimanager")
local VerticalGroup = require("ui/widget/verticalgroup")
local VerticalSpan = require("ui/widget/verticalspan")
local Screen = Device.screen

local FACES = {
    meta = Font:getFace("smallinfofont", 18),
    summary = Font:getFace("cfont", 22),
    heading = Font:getFace("smalltfont", 20),
    body = Font:getFace("cfont", 19),
    note = Font:getFace("smallinfofont", 17),
}

local AskScreen = InputContainer:extend{
    covers_fullscreen = true,
    title = "",
    blocks = nil,
    footer_rows = nil,
    scroll_offset = nil,
    close_callback = nil,
}

function AskScreen:init()
    self.dimen = Geom:new{ x = 0, y = 0, w = Screen:getWidth(), h = Screen:getHeight() }
    local side = Size.padding.large
    local content_width = self.dimen.w - ScrollableContainer:getScrollbarWidth() - 2 * side
    local title_bar = TitleBar:new{
        fullscreen = true,
        width = self.dimen.w,
        align = "left",
        with_bottom_line = true,
        title = self.title,
        title_shrink_font_to_fit = true,
        close_callback = function() self:onClose() end,
        show_parent = self,
    }
    local footer = ButtonTable:new{
        width = self.dimen.w - 2 * side,
        buttons = self.footer_rows,
        zero_sep = true,
        show_parent = self,
    }
    local footer_block = VerticalGroup:new{
        align = "center",
        LineWidget:new{ dimen = Geom:new{ w = self.dimen.w, h = Size.line.medium }, background = Blitbuffer.COLOR_GRAY },
        VerticalSpan:new{ width = Size.padding.default },
        CenterContainer:new{ dimen = Geom:new{ w = self.dimen.w, h = footer:getSize().h }, footer },
        VerticalSpan:new{ width = Size.padding.default },
    }
    local content = VerticalGroup:new{ align = "left" }
    for _, block in ipairs(self.blocks) do
        table.insert(content, VerticalSpan:new{ width = Size.padding.large })
        table.insert(content, self:buildBlock({ block = block, width = content_width }))
    end
    table.insert(content, VerticalSpan:new{ width = Size.padding.large * 2 })
    local padded = HorizontalGroup:new{
        align = "top",
        HorizontalSpan:new{ width = side },
        content,
    }
    local crop_height = self.dimen.h - title_bar:getHeight() - footer_block:getSize().h
    self.cropping_widget = ScrollableContainer:new{
        dimen = Geom:new{ w = self.dimen.w, h = crop_height },
        show_parent = self,
        padded,
    }
    if self.scroll_offset then
        self.cropping_widget:initState()
        if self.cropping_widget._is_scrollable then
            local max_y = self.cropping_widget._max_scroll_offset_y
            self.cropping_widget:setScrolledOffset(Geom:new{ x = 0, y = math.min(self.scroll_offset.y, max_y) })
            self.cropping_widget:_updateScrollBars()
        end
    end
    self[1] = FrameContainer:new{
        width = self.dimen.w,
        height = self.dimen.h,
        padding = 0,
        margin = 0,
        bordersize = 0,
        background = Blitbuffer.COLOR_WHITE,
        VerticalGroup:new{
            align = "left",
            title_bar,
            self.cropping_widget,
            footer_block,
        },
    }
end

function AskScreen:text(args)
    return TextBoxWidget:new{
        text = args.text,
        face = FACES[args.style],
        width = args.width,
        fgcolor = (args.style == "meta" or args.style == "note") and Blitbuffer.COLOR_DARK_GRAY or Blitbuffer.COLOR_BLACK,
    }
end

function AskScreen:button(args)
    return Button:new{
        text = args.item.text,
        width = args.width,
        align = "left",
        enabled = args.item.enabled ~= false,
        text_font_bold = args.item.bold == true,
        text_font_size = 19,
        radius = Size.radius.button,
        callback = args.item.callback,
        show_parent = self,
    }
end

function AskScreen:buildBlock(args)
    local group = VerticalGroup:new{ align = "left" }
    local indent = Size.padding.large
    for _, item in ipairs(args.block) do
        if item.kind == "text" then
            table.insert(group, self:text({ text = item.text, style = item.style, width = args.width }))
        elseif item.kind == "separator" then
            table.insert(group, LineWidget:new{ dimen = Geom:new{ w = args.width, h = Size.line.thin }, background = Blitbuffer.COLOR_GRAY })
        elseif item.kind == "button" then
            table.insert(group, self:button({ item = item, width = args.width }))
            if item.description and item.description ~= "" then
                table.insert(group, HorizontalGroup:new{
                    align = "top",
                    HorizontalSpan:new{ width = indent },
                    self:text({ text = item.description, style = "note", width = args.width - indent }),
                })
            end
        end
        table.insert(group, VerticalSpan:new{ width = Size.padding.default })
    end
    return group
end

function AskScreen:getScrollOffset()
    return self.cropping_widget:getScrolledOffset()
end

function AskScreen:onShow()
    UIManager:setDirty(self, function() return "ui", self.dimen end)
end

function AskScreen:onClose()
    UIManager:close(self)
    if self.close_callback then
        self.close_callback()
    end
    return true
end

return AskScreen
