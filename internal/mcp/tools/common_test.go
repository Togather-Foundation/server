package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestNormalizeArgs_CamelCaseAliasesCopiedToSnake(t *testing.T) {
	args := map[string]any{
		"startDate": "2026-08-01",
		"endDate":   "2026-08-31",
		"q":         "jazz",
		"after":     "cursor123",
		"nearLat":   float64(43.6656),
		"nearLon":   float64(-79.4113),
		"radiusKm":  float64(5),
	}

	normalizeArgs(args)

	require.Equal(t, "2026-08-01", args["start_date"])
	require.Equal(t, "2026-08-31", args["end_date"])
	require.Equal(t, "jazz", args["query"])
	require.Equal(t, "cursor123", args["cursor"])
	require.Equal(t, float64(43.6656), args["near_lat"])
	require.Equal(t, float64(-79.4113), args["near_lon"])
	require.Equal(t, float64(5), args["radius"])
}

func TestNormalizeArgs_SnakeCaseWinsOverCamelCase(t *testing.T) {
	args := map[string]any{
		"start_date": "2026-09-01",
		"startDate":  "2026-08-01",
	}

	normalizeArgs(args)

	require.Equal(t, "2026-09-01", args["start_date"], "explicit snake_case should win")
}

func TestNormalizeArgs_NoSpuriousKeys(t *testing.T) {
	args := map[string]any{
		"city":  "Toronto",
		"limit": float64(10),
		"types": "event",
	}

	normalizeArgs(args)

	require.Len(t, args, 3)
	require.Equal(t, "Toronto", args["city"])
	require.Equal(t, float64(10), args["limit"])
	require.Equal(t, "event", args["types"])
}

func TestUnmarshalArgs_AcceptsCamelCaseRESTAliases(t *testing.T) {
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"startDate": "2026-08-01",
				"endDate":   "2026-08-31",
				"q":         "jazz",
				"limit":     float64(25),
			},
		},
	}

	args := struct {
		Query     string `json:"query"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Limit     int    `json:"limit"`
	}{}

	require.NoError(t, unmarshalArgs(request, &args))
	require.Equal(t, "jazz", args.Query)
	require.Equal(t, "2026-08-01", args.StartDate)
	require.Equal(t, "2026-08-31", args.EndDate)
	require.Equal(t, 25, args.Limit)
}

func TestUnmarshalArgs_NilArguments(t *testing.T) {
	request := mcp.CallToolRequest{}

	args := struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}{Limit: 50}

	require.NoError(t, unmarshalArgs(request, &args))
	require.Equal(t, 50, args.Limit)
}
