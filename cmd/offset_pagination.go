package cmd

import (
	"fmt"
	"net/http"
	"strconv"
)

type offsetPagination struct {
	HasMore    bool
	NextOffset int
}

func parseOffsetPagination(response *http.Response, offset int64) (offsetPagination, error) {
	if response == nil {
		return offsetPagination{}, fmt.Errorf("response is missing pagination headers")
	}
	hasMoreValue := response.Header.Get("X-Has-More")
	hasMore, err := strconv.ParseBool(hasMoreValue)
	if err != nil {
		return offsetPagination{}, fmt.Errorf("invalid X-Has-More header %q", hasMoreValue)
	}

	nextOffsetValue := response.Header.Get("X-Next-Offset")
	nextOffset := 0
	if nextOffsetValue != "" || hasMore {
		nextOffset, err = strconv.Atoi(nextOffsetValue)
	}
	if err != nil || nextOffset < 0 {
		return offsetPagination{}, fmt.Errorf("invalid X-Next-Offset header %q", nextOffsetValue)
	}
	if hasMore && int64(nextOffset) <= offset {
		return offsetPagination{}, fmt.Errorf("X-Has-More is true but X-Next-Offset does not advance the current offset")
	}
	if !hasMore && nextOffset != 0 {
		return offsetPagination{}, fmt.Errorf("X-Has-More is false but X-Next-Offset is %d", nextOffset)
	}

	return offsetPagination{HasMore: hasMore, NextOffset: nextOffset}, nil
}
