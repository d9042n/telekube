package telegram

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPaginator_DefaultPageSize(t *testing.T) {
	t.Parallel()

	p := NewPaginator([]string{"a", "b"}, 0)
	assert.Equal(t, 8, p.PageSize)
}

func TestPaginator_TotalPages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		itemCount int
		pageSize  int
		expected  int
	}{
		{"empty", 0, 5, 1},
		{"exact fit", 10, 5, 2},
		{"with remainder", 11, 5, 3},
		{"single page", 3, 5, 1},
		{"one item", 1, 5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			items := make([]string, tt.itemCount)
			p := NewPaginator(items, tt.pageSize)
			assert.Equal(t, tt.expected, p.TotalPages())
		})
	}
}

func TestPaginator_Page(t *testing.T) {
	t.Parallel()

	items := []string{"a", "b", "c", "d", "e", "f", "g"}
	p := NewPaginator(items, 3)

	// Page 1
	p.Current = 1
	page, hasMore := p.Page()
	assert.Equal(t, []string{"a", "b", "c"}, page)
	assert.True(t, hasMore)

	// Page 2
	p.Current = 2
	page, hasMore = p.Page()
	assert.Equal(t, []string{"d", "e", "f"}, page)
	assert.True(t, hasMore)

	// Page 3 (last)
	p.Current = 3
	page, hasMore = p.Page()
	assert.Equal(t, []string{"g"}, page)
	assert.False(t, hasMore)
}

func TestPaginator_Page_OutOfRange(t *testing.T) {
	t.Parallel()

	items := []string{"a", "b"}
	p := NewPaginator(items, 5)

	p.Current = 10
	page, hasMore := p.Page()
	assert.Nil(t, page)
	assert.False(t, hasMore)
}

func TestPaginator_Page_Empty(t *testing.T) {
	t.Parallel()

	p := NewPaginator(nil, 5)

	p.Current = 1
	page, hasMore := p.Page()
	assert.Nil(t, page)
	assert.False(t, hasMore)
}

func TestPaginator_Keyboard(t *testing.T) {
	t.Parallel()

	items := make([]string, 20)
	for i := range items {
		items[i] = "item"
	}
	p := NewPaginator(items, 5)

	// Page 1: should have "Next" but not "Prev"
	p.Current = 1
	kb := p.Keyboard("test")
	assert.NotNil(t, kb)

	// Page 2: should have both "Prev" and "Next"
	p.Current = 2
	kb = p.Keyboard("test")
	assert.NotNil(t, kb)

	// Page 4 (last): should have "Prev" but not "Next"
	p.Current = 4
	kb = p.Keyboard("test")
	assert.NotNil(t, kb)
}
