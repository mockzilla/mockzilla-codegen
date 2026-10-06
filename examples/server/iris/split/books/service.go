// Written once by mockzilla-codegen. Edit it freely: generate does not overwrite it.

package books

import (
	"context"
	"errors"

	"github.com/mockzilla/mockzilla-codegen/examples/server/iris/split/api"
)

// ErrNotImplemented is what every operation returns until it is written.
var ErrNotImplemented = errors.New("not implemented")

var _ api.BooksInterface = (*Books)(nil)

// Books implements api.BooksInterface.
type Books struct{}

// NewBooks returns a Books.
func NewBooks() *Books {
	return &Books{}
}

// GetBook handles GET /books/{isbn}.
func (s *Books) GetBook(ctx context.Context, opts *api.GetBookServiceRequestOptions) (*api.GetBookResponseData, error) {
	return nil, ErrNotImplemented
}
