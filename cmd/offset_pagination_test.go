package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOffsetPagination(t *testing.T) {
	for _, tc := range []struct {
		name, hasMore, nextOffset string
		offset                    int64
		want                      offsetPagination
		wantErr                   string
	}{
		{name: "more results", hasMore: "true", nextOffset: "120", offset: 100, want: offsetPagination{HasMore: true, NextOffset: 120}},
		{name: "terminal page", hasMore: "false", nextOffset: "0", offset: 100},
		{name: "omitted terminal offset", hasMore: "false", offset: 100},
		{name: "missing has more", nextOffset: "120", wantErr: "invalid X-Has-More"},
		{name: "invalid has more", hasMore: "next", wantErr: "invalid X-Has-More"},
		{name: "missing next offset", hasMore: "true", wantErr: "invalid X-Next-Offset"},
		{name: "invalid next offset", hasMore: "true", nextOffset: "next", wantErr: "invalid X-Next-Offset"},
		{name: "negative next offset", hasMore: "true", nextOffset: "-1", wantErr: "invalid X-Next-Offset"},
		{name: "overflow", hasMore: "true", nextOffset: "999999999999999999999", wantErr: "invalid X-Next-Offset"},
		{name: "zero next offset", hasMore: "true", nextOffset: "0", wantErr: "does not advance"},
		{name: "same offset", hasMore: "true", nextOffset: "100", offset: 100, wantErr: "does not advance"},
		{name: "backwards offset", hasMore: "true", nextOffset: "20", offset: 100, wantErr: "does not advance"},
		{name: "terminal with cursor", hasMore: "false", nextOffset: "120", wantErr: "X-Has-More is false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &http.Response{Header: make(http.Header)}
			if tc.hasMore != "" {
				response.Header.Set("X-Has-More", tc.hasMore)
			}
			if tc.nextOffset != "" {
				response.Header.Set("X-Next-Offset", tc.nextOffset)
			}
			got, err := parseOffsetPagination(response, tc.offset)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	_, err := parseOffsetPagination(nil, 0)
	require.ErrorContains(t, err, "missing pagination headers")
}

func TestOffsetPaginationListCommands(t *testing.T) {
	for _, command := range []string{"projects", "vaults", "vault-provider-configs"} {
		for _, hasMore := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/more=%t", command, hasMore), func(t *testing.T) {
				client := vaultTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "20", r.URL.Query().Get("offset"))
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Has-More", fmt.Sprint(hasMore))
					if hasMore {
						w.Header().Set("X-Next-Offset", "20")
					}
					_, _ = io.WriteString(w, "[]")
				})
				var err error
				out := captureStdout(t, func() {
					switch command {
					case "projects":
						err = (ProjectsCmd{projects: &client.Projects}).List(context.Background(), ProjectsListInput{Limit: 20, Offset: 20, Output: "json"})
					case "vaults":
						err = (VaultsCmd{vaults: &client.Vaults}).List(context.Background(), 20, 20, "", "json")
					case "vault-provider-configs":
						err = (VaultProviderConfigsCmd{configs: &client.VaultProviderConfigs}).List(context.Background(), 20, 20, "json")
					}
				})
				if hasMore {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					assert.NotContains(t, out, "next_offset")
				}
			})
		}
	}
}
