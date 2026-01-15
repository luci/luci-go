// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanutil

import (
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/errors"
)

// Iterator is a generic iterator over rows in Spanner.
//
// Under the hood, it queries rows page-wise for performance and to avoid
// demanding too large result sets from Spanner. This happens transparently
// to the user.
//
// R is the type of the row being iterated over.
// I is the type of the ID (page token) used for pagination.
type Iterator[R any, I any] struct {
	// queryFn is a function that returns a new page iterator with the
	// given page size.
	queryFn func(pageToken I) (*PageIterator[R], error)
	// The page token. This is set to the ID of the last row returned.
	pageToken I
	// idAccessor extracts the ID from a row.
	idAccessor func(R) I

	// iterator is the current result iterator. This may be nil
	// if Next has not been called yet.
	iterator *PageIterator[R]
	// done is set to true once all results have been returned
	// and there is nothing more to read.
	done bool
}

// NewIterator initialises a new generic row iterator.
func NewIterator[R any, I any](queryFn func(pageToken I) (*PageIterator[R], error), idAccessor func(R) I, pageToken I) *Iterator[R, I] {
	return &Iterator[R, I]{
		queryFn:    queryFn,
		idAccessor: idAccessor,
		pageToken:  pageToken,
	}
}

// Next returns the next row in the iteration.
// If there are no more rows, iterator.Done is returned.
func (i *Iterator[R, I]) Next() (R, error) {
	var empty R
	if i.done {
		return empty, iterator.Done
	}
	if i.iterator == nil {
		// Try to construct the first iterator.
		var err error
		i.iterator, err = i.queryFn(i.pageToken)
		if err != nil {
			return empty, fmt.Errorf("query first page: %w", err)
		}
	}

	row, err := i.iterator.Next()
	if err == iterator.Done {
		if i.iterator.Exhausted() {
			i.done = true
			return empty, iterator.Done
		}
		// This iterator has been consumed, but there may be more pages.
		// Clean up the current iterator.
		i.iterator.Stop()

		// Query the next page.
		i.iterator, err = i.queryFn(i.pageToken)
		if err != nil {
			return empty, fmt.Errorf("query next page: %w", err)
		}
		row, err = i.iterator.Next()
		if err == iterator.Done {
			// New iterator is immediately exhausted. This can happen if the
			// last iterator had a full page of results (so thought there may
			// be more results), but actually there were no more results.
			if !i.iterator.Exhausted() {
				return empty, fmt.Errorf("next page iterator is not exhausted despite being done immediately")
			}
			i.done = true
			return empty, iterator.Done
		}
		// Continue iterating using new iterator.
	}
	if err != nil {
		return empty, err
	}
	i.pageToken = i.idAccessor(row)
	return row, nil
}

// PageToken returns the current page token. This can be used to resume queries
// at the current position.
// If the returned page token is the zero value of I, then the last result has been consumed.
// This method must only be called after calling Next() at least once.
func (it *Iterator[R, I]) PageToken() I {
	if it.done {
		var empty I
		return empty
	}
	return it.pageToken
}

// Stop should be called after you finish using the iterator.
func (it *Iterator[R, I]) Stop() {
	if it.iterator != nil {
		it.iterator.Stop()
	}
}

// PageIterator represents an iterator over a page of rows
// returned by a Spanner query.
//
// The Spanner query has a defined LIMIT @pageSize clause to constrain
// the result set size, which this iterator will track. If the number
// of rows returned by the Spanner iterator ends up being less than
// the requested page size, Exhausted() will become true.
// This indicates there are no more pages of rows to fetch.
type PageIterator[R any] struct {
	iterator *spanner.RowIterator
	decoder  func(*spanner.Row) (R, error)

	// done tracks whether all rows have been consumed from this iterator
	// (via Next()).
	done bool

	// rowsConsumed counts the number of rows consumed form this iterator
	// (via Next()).
	rowsConsumed int
	// pageSize is the maximum size of the page this iterator is iteratring
	// over. It corresponds to the LIMIT of the underlying query.
	pageSize int
}

// NewPageIterator initialises a new PageIterator.
func NewPageIterator[R any](iterator *spanner.RowIterator, decoder func(*spanner.Row) (R, error), pageSize int) *PageIterator[R] {
	return &PageIterator[R]{
		iterator: iterator,
		decoder:  decoder,
		pageSize: pageSize,
	}
}

// Next reads and consumes the next result from the iterator.
// If there are no more results, iterator.Done is returned.
func (i *PageIterator[R]) Next() (R, error) {
	var empty R
	if i.done {
		return empty, iterator.Done
	}
	row, err := i.decodeNext()
	if err != nil {
		if err == iterator.Done {
			i.done = true
		}
		return empty, err
	}
	i.rowsConsumed++
	return row, nil
}

// Exhausted returns true if all rows have been consumed from this iterator,
// and there are no more pages of results that could be fetched.
func (i *PageIterator[R]) Exhausted() bool {
	return i.done && i.rowsConsumed < i.pageSize
}

// Stop terminates the iteration. It should be called after you finish using the iterator.
func (i *PageIterator[R]) Stop() {
	i.iterator.Stop()
}

// decodeNext reads the next row from the iterator.
func (i *PageIterator[R]) decodeNext() (R, error) {
	var empty R
	spanRow, err := i.iterator.Next()
	if err == iterator.Done {
		return empty, iterator.Done
	}
	if err != nil {
		return empty, errors.Fmt("obtain next row: %w", err)
	}
	return i.decoder(spanRow)
}

// PeekingIterator is a wrapper around an iterator that allows peeking at the next item.
type PeekingIterator[R any] struct {
	iter SourceIterator[R]

	// peeked stores the next element if hasPeeked is true.
	peeked R
	// peekedErr stores the error from fetching the next element if hasPeeked is true.
	peekedErr error
	// hasPeeked indicates if we have a buffered element/error.
	hasPeeked bool
}

// SourceIterator is the interface required by PeekingIterator.
type SourceIterator[R any] interface {
	// Next consumes the next item from the iterator.
	// If there are no more results, iterator.Done is returned.
	Next() (R, error)
	// Stop terminates the iteration. It should be called after you finish using the iterator.
	Stop()
}

// NewPeekingIterator creates a new PeekingIterator.
func NewPeekingIterator[R any](iter SourceIterator[R]) *PeekingIterator[R] {
	return &PeekingIterator[R]{
		iter: iter,
	}
}

// Next returns the next element.
func (i *PeekingIterator[R]) Next() (R, error) {
	if i.hasPeeked {
		i.hasPeeked = false
		return i.peeked, i.peekedErr
	}
	return i.iter.Next()
}

// Peek returns the next element without advancing the iterator.
func (i *PeekingIterator[R]) Peek() (R, error) {
	if !i.hasPeeked {
		i.peeked, i.peekedErr = i.iter.Next()
		i.hasPeeked = true
	}
	return i.peeked, i.peekedErr
}

// Stop terminates the iteration. It should be called after you finish using the iterator.
func (i *PeekingIterator[R]) Stop() {
	i.iter.Stop()
}
