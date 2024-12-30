package btc

import (
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
)

func TestClient_GetMiningInfo(t *testing.T) {
	tt := []struct {
		name string
		port int
	}{
		{
			name: "18332",
			port: 18332,
		},
		{
			name: "18443",
			port: 18443,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			const (
				rpcUser        = "bitcoin"
				rpcPassword    = "bitcoin"
				rpcHostDefault = "localhost"
			)
			client, err := New(rpcHostDefault, tc.port, rpcUser, rpcPassword, slog.Default())
			require.NoError(t, err)

			miningInfo, err := client.GetMiningInfo()
			require.NoError(t, err)
			require.NotNil(t, miningInfo)

			blockHash, err := client.GenerateToAddress(1, "")
			require.NoError(t, err)

			require.NotNil(t, blockHash)
		})
	}
}
