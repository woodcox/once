package ui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/mouse"
)

var formKeys = struct {
	Tab      key.Binding
	ShiftTab key.Binding
	Enter    key.Binding
}{
	Tab:      NewKeyBinding("tab"),
	ShiftTab: NewKeyBinding("shift+tab"),
	Enter:    NewKeyBinding("enter"),
}

type FormField interface {
	Update(tea.Msg) tea.Cmd
	View() string
	Focus() tea.Cmd
	Blur()
	SetWidth(int)
	IsFocusable() bool
}

type valuer interface {
	Value() string
}

// TextField

type TextField struct {
	input      textinput.Model
	digitsOnly bool
}

func NewTextField(placeholder string) *TextField {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.CharLimit = 256
	return &TextField{input: input}
}

func (f *TextField) Value() string {
	return f.input.Value()
}

func (f *TextField) SetValue(v string) {
	f.input.SetValue(v)
}

func (f *TextField) SetPlaceholder(p string) {
	f.input.Placeholder = p
}

func (f *TextField) SetCharLimit(n int) {
	f.input.CharLimit = n
}

func (f *TextField) SetDigitsOnly(v bool) {
	f.digitsOnly = v
}

func (f *TextField) SetEchoPassword() {
	f.input.EchoMode = textinput.EchoPassword
}

func (f *TextField) Update(msg tea.Msg) tea.Cmd {
	if f.digitsOnly {
		if msg, ok := msg.(tea.KeyPressMsg); ok {
			if k := msg.Key(); k.Text != "" && !isDigitKey(k) {
				return nil
			}
		}
	}

	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd
}

func (f *TextField) View() string {
	return f.input.View()
}

func (f *TextField) Focus() tea.Cmd {
	return f.input.Focus()
}

func (f *TextField) Blur() {
	f.input.Blur()
}

func (f *TextField) SetWidth(w int) {
	f.input.SetWidth(w)
}

func (f *TextField) IsFocusable() bool { return true }

// CheckboxField

type CheckboxField struct {
	label      string
	checked    bool
	disabledFn func() (disabled bool, text string)
}

func NewCheckboxField(label string, checked bool) *CheckboxField {
	return &CheckboxField{label: label, checked: checked}
}

func (f *CheckboxField) Checked() bool {
	return f.checked
}

func (f *CheckboxField) SetDisabledWhen(fn func() (disabled bool, text string)) {
	f.disabledFn = fn
}

func (f *CheckboxField) Toggle() {
	if f.disabledFn != nil {
		if disabled, _ := f.disabledFn(); disabled {
			return
		}
	}
	f.checked = !f.checked
}

func (f *CheckboxField) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if msg.String() == "space" || msg.String() == " " {
			f.Toggle()
		}
	}
	return nil
}

func (f *CheckboxField) View() string {
	if f.disabledFn != nil {
		if disabled, text := f.disabledFn(); disabled {
			return text
		}
	}

	if f.checked {
		return "[✓] " + f.label
	}
	return "[ ] " + f.label
}

func (f *CheckboxField) Focus() tea.Cmd    { return nil }
func (f *CheckboxField) Blur()             {}
func (f *CheckboxField) SetWidth(int)      {}
func (f *CheckboxField) IsFocusable() bool { return true }

// StaticField

type StaticField struct {
	value   string
	styleFn func(string) string
}

func NewStaticField(value string, styleFn func(string) string) *StaticField {
	return &StaticField{value: value, styleFn: styleFn}
}

func (f *StaticField) Value() string {
	return f.value
}

func (f *StaticField) SetValue(v string) {
	f.value = v
}

func (f *StaticField) Update(tea.Msg) tea.Cmd { return nil }
func (f *StaticField) View() string           { return f.styleFn(f.value) }
func (f *StaticField) Focus() tea.Cmd         { return nil }
func (f *StaticField) Blur()                  {}
func (f *StaticField) SetWidth(int)           {}
func (f *StaticField) IsFocusable() bool      { return false }

// FormActionButton

type FormActionButton struct {
	Label   string
	OnPress func() tea.Msg
}

// Form

type FormItem struct {
	Label       string
	Description string
	LabelTag    string
	Required    bool
	Field       FormField
}

// markerStyle styles the "(required)"/"(optional)" marker that follows a
// label. Fields naming a docktail key get the focus orange, matching the F1
// help panel; every other form keeps the muted border colour.
var markerStyle = lipgloss.NewStyle().Foreground(Colors.Border)

// tagStyle styles the docktail key a field maps to, in the primary colour so
// it stands apart from the field name beside it.
var tagStyle = lipgloss.NewStyle().Foreground(Colors.Primary)

// labelStyle is the style for the field name itself: the focus colour on the
// Tailscale screens, the default bold label everywhere else.
func (item FormItem) labelStyle() lipgloss.Style {
	if item.LabelTag == "" {
		return Styles.Label
	}
	return lipgloss.NewStyle().Bold(true).Foreground(Colors.Focused)
}

type FormSubmitMsg struct{}
type FormCancelMsg struct{}
type FormActionMsg struct{ Msg tea.Msg }

type Form struct {
	items        []FormItem
	submitLabel  string
	actionButton *FormActionButton
	focused      int
	width        int
	height       int
	wheelSteps   int
	errorField   int
	error        string
	onSubmit     func(f *Form) tea.Cmd
	onCancel     func(f *Form) tea.Cmd
	onRebuild    func(f *Form)
}

func NewForm(submitLabel string, items ...FormItem) Form {
	f := Form{
		items:       items,
		submitLabel: submitLabel,
	}

	for i, item := range items {
		if item.Field.IsFocusable() {
			f.focused = i
			item.Field.Focus()
			break
		}
	}

	return f
}

func (f Form) Init() tea.Cmd {
	if f.focused < len(f.items) {
		return f.items[f.focused].Field.Focus()
	}
	return nil
}

func (f Form) Update(msg tea.Msg) (Form, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		f.width = msg.Width
		f.height = msg.Height
		inputWidth := min(f.width-4, 60)
		for _, item := range f.items {
			item.Field.SetWidth(inputWidth)
		}

	case MouseEvent:
		if msg.IsClick {
			return f.handleClick(msg.Target)
		}

	case tea.MouseWheelMsg:
		f.scrollWheel(msg)
		return f, nil

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, formKeys.Tab):
			return f.focusNext()
		case key.Matches(msg, formKeys.ShiftTab):
			return f.focusPrev()
		case key.Matches(msg, formKeys.Enter):
			return f.handleEnter()
		}
	}

	if f.focused < len(f.items) {
		_, isKey := msg.(tea.KeyPressMsg)
		_, isPaste := msg.(tea.PasteMsg)
		if isKey {
			f.clearErrorOnInput()
		}
		cmd := f.items[f.focused].Field.Update(msg)
		if (isKey || isPaste) && f.onRebuild != nil {
			f.onRebuild(&f)
			f.focused = min(f.focused, f.totalCount()-1)
		}
		return f, cmd
	}

	return f, nil
}

func (f Form) View() string {
	blocks := f.itemBlocks()
	buttons := f.renderButtons()

	// A form that doesn't fit the terminal shows whole items around the
	// focused one and keeps the buttons reachable.
	if f.height > 0 && f.height < 3 {
		return f.convergeBlock(blocks, buttons)
	}

	start, end, windowed := f.visibleRange(blocks)
	if !windowed {
		parts := make([]string, 0, len(blocks)+2)
		parts = append(parts, blocks...)
		parts = append(parts, "", buttons)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	start, end = f.applyWindowStart(start, end)

	parts := make([]string, 0, end-start+3)
	if start > 0 {
		parts = append(parts, hiddenFieldsIndicator("↑", start))
	}
	parts = append(parts, blocks[start:end]...)
	if end < len(blocks) {
		parts = append(parts, hiddenFieldsIndicator("↓", len(blocks)-end))
	}
	parts = append(parts, buttons)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// convergeBlock renders the focused field's input on a single line, plus the
// buttons if they fit. Keystrokes still go to the focused field; this just makes
// sure the field the user is editing is visible at all.
func (f Form) convergeBlock(blocks []string, buttons string) string {
	if f.focused >= len(blocks) {
		return buttons
	}
	lines := strings.Split(blocks[f.focused], "\n")
	// Each block is: [label, input_top, input_content, input_bottom, blank].
	content := lines[2]
	if f.height >= 4 {
		return lipgloss.JoinVertical(lipgloss.Left, content, buttons)
	}
	return content
}

func (f *Form) SetActionButton(label string, onPress func() tea.Msg) {
	f.actionButton = &FormActionButton{Label: label, OnPress: onPress}
}

func (f *Form) OnSubmit(fn func(f *Form) tea.Cmd) {
	f.onSubmit = fn
}

func (f *Form) OnCancel(fn func(f *Form) tea.Cmd) {
	f.onCancel = fn
}

func (f *Form) OnRebuild(fn func(f *Form)) {
	f.onRebuild = fn
}

func (f *Form) AppendItems(items ...FormItem) {
	inputWidth := min(f.width-4, 60)
	for _, item := range items {
		item.Field.SetWidth(inputWidth)
	}
	f.items = append(f.items, items...)
}

// PruneItems drops the items the predicate reports as empty, moving focus off
// a removed item to the one that takes its place (or the last remaining item).
// Used by forms that build their fields from what the user has typed.
func (f *Form) PruneItems(empty func(i int, item FormItem) bool) {
	kept := make([]FormItem, 0, len(f.items))
	removed := 0
	focusRemoved := false

	for i, item := range f.items {
		if !empty(i, item) {
			kept = append(kept, item)
			continue
		}
		if i <= f.focused {
			removed++
			focusRemoved = i == f.focused
		}
	}

	f.items = kept
	switch {
	case removed == 0:
		return
	case focusRemoved:
		// Focus lands on whatever shifted into the removed slot, or on the
		// last item when the removed slot was the tail.
		f.focused = min(f.focused-removed+1, len(f.items))
	default:
		f.focused = max(0, f.focused-removed)
	}
	f.focusCurrent()
}

func (f Form) ItemCount() int {
	return len(f.items)
}

func (f Form) Focused() int {
	return f.focused
}

func (f Form) Field(i int) FormField {
	return f.items[i].Field
}

func (f Form) TextField(i int) *TextField {
	return f.items[i].Field.(*TextField)
}

func (f Form) CheckboxField(i int) *CheckboxField {
	return f.items[i].Field.(*CheckboxField)
}

func (f Form) HasError() bool {
	return f.error != ""
}

func (f Form) Error() string {
	return f.error
}

// Private

func (f Form) itemBlocks() []string {
	blocks := make([]string, 0, len(f.items))
	errorStyle := lipgloss.NewStyle().Foreground(Colors.Error)

	for i, item := range f.items {
		if _, isStatic := item.Field.(*StaticField); isStatic {
			blocks = append(blocks, item.Field.View())
			continue
		}

		label := item.Label
		requiredMarker := strings.HasSuffix(item.Label, "(required)") || strings.HasSuffix(item.Label, "(optional)")
		switch {
		case item.Required && !requiredMarker:
			label += markerStyle.Render(" (required)")
		case !item.Required && item.Field.IsFocusable() && !requiredMarker:
			label += markerStyle.Render(" (optional)")
		}
		if item.LabelTag != "" {
			label += markerStyle.Render(" — ") + tagStyle.Render(item.LabelTag)
		}

		parts := []string{item.labelStyle().Render(label)}

		hasError := f.error != "" && i == f.errorField
		inputStyle := Styles.WithError(Styles.Focus(Styles.Input, f.focused == i), hasError)
		parts = append(parts, mouse.Mark(fieldTarget(i), inputStyle.Render(item.Field.View())))

		if item.Description != "" {
			parts = append(parts, Styles.MutedText.Render(item.Description))
		}
		if hasError {
			parts = append(parts, errorStyle.Render(f.error))
		}
		parts = append(parts, "")

		blocks = append(blocks, lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	return blocks
}

func (f Form) renderButtons() string {
	submitButton := mouse.Mark("submit", Styles.Focus(Styles.ButtonPrimary, f.focused == f.submitIndex()).
		Render(f.submitLabel))

	buttonParts := []string{submitButton}

	if f.actionButton != nil {
		actionBtn := mouse.Mark("action", Styles.Focus(Styles.Button, f.focused == f.actionIndex()).
			Render(f.actionButton.Label))
		buttonParts = append(buttonParts, actionBtn)
	}

	cancelButton := mouse.Mark("cancel", Styles.Focus(Styles.Button, f.focused == f.cancelIndex()).
		Render("Cancel"))

	buttonParts = append(buttonParts, cancelButton)
	return lipgloss.JoinHorizontal(lipgloss.Center, buttonParts...)
}

// visibleRange returns the slice of item blocks to render. When the form is
// taller than the terminal it grows the window around the focused item, one
// whole block at a time, and reserves a line for each hidden-fields indicator.
func (f Form) visibleRange(blocks []string) (start, end int, windowed bool) {
	n := len(blocks)
	if f.height <= 0 || n == 0 {
		return 0, n, false
	}

	total := 0
	for _, block := range blocks {
		total += lipgloss.Height(block)
	}
	if total+2 <= f.height {
		return 0, n, false
	}

	focused := min(max(f.focused, 0), n-1)
	start, end, _ = growWindow(blocks, focused, f.height)
	return start, end, true
}

// scrollWheel nudges the visible window of item blocks without touching focus,
// so wheeling over a form never jumps the cursor into a hidden field.
func (f *Form) scrollWheel(msg tea.MouseWheelMsg) {
	if f.height <= 0 || len(f.items) == 0 {
		return
	}
	if _, _, windowed := f.visibleRange(f.itemBlocks()); !windowed {
		return
	}

	switch msg.Button {
	case tea.MouseWheelUp:
		f.wheelSteps--
	case tea.MouseWheelDown:
		f.wheelSteps++
	default:
		return
	}
}

// applyWindowStart renders the window the keyboard chose, shifted by however
// many wheel steps the user has taken since. The focused item is always in the
// window: the shift is clamped to the slack between the focused item and the
// window edge.
func (f Form) applyWindowStart(start, end int) (int, int) {
	blocks := f.itemBlocks()
	if f.wheelSteps == 0 {
		return start, end
	}

	shift := f.wheelSteps
	if start+shift > f.focused {
		shift = f.focused - start
	}
	if end+shift <= f.focused {
		shift = f.focused - end + 1
	}

	shifted, shiftedEnd, _ := growWindow(blocks, start+shift, f.height)
	if shiftedEnd-start+shift < end-start || shifted > f.focused {
		return start, end
	}
	return shifted, shiftedEnd
}

func (f Form) focusNext() (Form, tea.Cmd) {
	f.blurCurrent()
	f.wheelSteps = 0
	f.focused = (f.focused + 1) % f.totalCount()
	return f.focusToNextFocusable(1)
}

func (f Form) focusPrev() (Form, tea.Cmd) {
	f.blurCurrent()
	f.wheelSteps = 0
	f.focused = (f.focused - 1 + f.totalCount()) % f.totalCount()
	return f.focusToNextFocusable(-1)
}

func (f Form) blurCurrent() {
	if f.focused < len(f.items) {
		f.items[f.focused].Field.Blur()
	}
}

func (f Form) focusCurrent() tea.Cmd {
	if f.focused < len(f.items) {
		return f.items[f.focused].Field.Focus()
	}
	return nil
}

// focusToNextFocusable walks the focusable positions in the given direction (+1
// or -1) and stops on the first one it finds, so that shift+tab from a button
// reaches the last focusable field even when the form ends with non-focusable
// decorations like a label preview.
func (f Form) focusToNextFocusable(direction int) (Form, tea.Cmd) {
	total := f.totalCount()
	if total == 0 {
		return f, nil
	}
	start := f.focused
	for range total {
		if f.focused >= len(f.items) {
			return f, nil
		}
		if f.items[f.focused].Field.IsFocusable() {
			return f, f.focusCurrent()
		}
		f.focused = (f.focused + direction + total) % total
		if f.focused == start {
			return f, nil
		}
	}
	return f, nil
}

func (f Form) handleEnter() (Form, tea.Cmd) {
	switch {
	case f.focused < len(f.items):
		return f.focusNext()
	case f.actionButton != nil && f.focused == f.actionIndex():
		return f, func() tea.Msg { return f.actionButton.OnPress() }
	case f.focused == f.submitIndex():
		return f.submitIfValid()
	case f.focused == f.cancelIndex():
		if f.onCancel != nil {
			return f, f.onCancel(&f)
		}
		return f, nil
	}
	return f, nil
}

func (f Form) handleClick(target string) (Form, tea.Cmd) {
	if target == "" {
		return f, nil
	}

	for i := range f.items {
		if target == fieldTarget(i) {
			if cb, ok := f.items[i].Field.(*CheckboxField); ok {
				cb.Toggle()
			}
			return f.focusIndex(i)
		}
	}

	switch target {
	case "submit":
		f.blurCurrent()
		f.focused = f.submitIndex()
		return f.submitIfValid()
	case "action":
		if f.actionButton != nil {
			f.blurCurrent()
			f.focused = f.actionIndex()
			return f, func() tea.Msg { return f.actionButton.OnPress() }
		}
	case "cancel":
		f.blurCurrent()
		f.focused = f.cancelIndex()
		if f.onCancel != nil {
			return f, f.onCancel(&f)
		}
	}

	return f, nil
}

func (f *Form) clearErrorOnInput() {
	if f.error != "" && f.focused == f.errorField {
		f.error = ""
	}
}

func (f Form) submitIfValid() (Form, tea.Cmd) {
	var valid bool
	f, valid = f.validate()
	if !valid {
		return f.focusIndex(f.errorField)
	}
	if f.onSubmit != nil {
		return f, f.onSubmit(&f)
	}
	return f, nil
}

func (f Form) validate() (Form, bool) {
	f.error = ""

	for i, item := range f.items {
		if !item.Required || !item.Field.IsFocusable() {
			continue
		}
		if v, ok := item.Field.(valuer); ok {
			if strings.TrimSpace(v.Value()) == "" {
				f.errorField = i
				f.error = item.Label + " is required"
				return f, false
			}
		}
	}

	return f, true
}

func (f Form) focusIndex(i int) (Form, tea.Cmd) {
	if i == f.focused {
		return f, nil
	}
	f.blurCurrent()
	f.focused = i
	f.wheelSteps = 0
	return f, f.focusCurrent()
}

func (f Form) submitIndex() int { return len(f.items) }

func (f Form) actionIndex() int { return len(f.items) + 1 }

func (f Form) cancelIndex() int {
	if f.actionButton != nil {
		return len(f.items) + 2
	}
	return len(f.items) + 1
}

func (f Form) totalCount() int {
	return f.cancelIndex() + 1
}

// Helpers

func clamp(v, low, high int) int {
	return min(max(v, low), high)
}

// growWindow returns the widest whole-block window around the anchor item that
// fits in the given height, reserving a line for each hidden-fields indicator.
func growWindow(blocks []string, anchor, height int) (start, end int, windowed bool) {
	n := len(blocks)
	anchor = clamp(anchor, 0, n-1)
	budget := height - 1

	start, end = anchor, anchor+1
	used := lipgloss.Height(blocks[anchor])

	for {
		grew := false

		if start > 0 {
			grown := used + lipgloss.Height(blocks[start-1])
			if grown+hiddenFieldsLines(start-1, end, n) <= budget {
				start--
				used = grown
				grew = true
			}
		}

		if end < n {
			grown := used + lipgloss.Height(blocks[end])
			if grown+hiddenFieldsLines(start, end+1, n) <= budget {
				end++
				used = grown
				grew = true
			}
		}

		if !grew {
			return start, end, true
		}
	}
}

func fieldTarget(i int) string {
	return "field:" + strconv.Itoa(i)
}

func isDigitKey(k tea.Key) bool {
	return unicode.IsDigit(rune(k.Text[0]))
}

func hiddenFieldsIndicator(arrow string, count int) string {
	return lipgloss.NewStyle().Foreground(Colors.Border).
		Render(fmt.Sprintf("%s %d more fields", arrow, count))
}

func hiddenFieldsLines(start, end, total int) int {
	lines := 0
	if start > 0 {
		lines++
	}
	if end < total {
		lines++
	}
	return lines
}

// formViewportHeight is the room a form has on a full-page screen: the title
// rule, status/blank lines above it, and the help line below it.
func formViewportHeight(terminalHeight int) int {
	return max(terminalHeight-5, 1)
}
