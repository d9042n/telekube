package telegram

import (
	"fmt"

	"gopkg.in/telebot.v3"
)

// Paginator manages paginated inline keyboard listings.
type Paginator struct {
	Items    []string
	PageSize int
	Current  int
}

// NewPaginator creates a paginator with the given items and page size.
func NewPaginator(items []string, pageSize int) *Paginator {
	if pageSize <= 0 {
		pageSize = 8
	}
	return &Paginator{
		Items:    items,
		PageSize: pageSize,
		Current:  1,
	}
}

// TotalPages returns the total number of pages.
func (p *Paginator) TotalPages() int {
	if len(p.Items) == 0 {
		return 1
	}
	total := len(p.Items) / p.PageSize
	if len(p.Items)%p.PageSize > 0 {
		total++
	}
	return total
}

// Page returns items for the current page.
func (p *Paginator) Page() (items []string, hasMore bool) {
	start := (p.Current - 1) * p.PageSize
	if start >= len(p.Items) {
		return nil, false
	}

	end := start + p.PageSize
	if end > len(p.Items) {
		end = len(p.Items)
	}

	return p.Items[start:end], end < len(p.Items)
}

// Keyboard creates a navigation keyboard with callback data prefixed by callbackPrefix.
func (p *Paginator) Keyboard(callbackPrefix string) *telebot.ReplyMarkup {
	menu := &telebot.ReplyMarkup{}

	var navButtons []telebot.Btn
	if p.Current > 1 {
		navButtons = append(navButtons, menu.Data(
			"◀️ Prev",
			fmt.Sprintf("%s_page", callbackPrefix),
			fmt.Sprintf("%d", p.Current-1),
		))
	}

	navButtons = append(navButtons, menu.Data(
		fmt.Sprintf("📄 %d/%d", p.Current, p.TotalPages()),
		fmt.Sprintf("%s_info", callbackPrefix),
	))

	if p.Current < p.TotalPages() {
		navButtons = append(navButtons, menu.Data(
			"▶️ Next",
			fmt.Sprintf("%s_page", callbackPrefix),
			fmt.Sprintf("%d", p.Current+1),
		))
	}

	rows := []telebot.Row{menu.Row(navButtons...)}
	menu.Inline(rows...)
	return menu
}
