package datafield

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDCTTransfer(t *testing.T) {
	t.Parallel()

	args := createMockArgumentsOperationParser()
	parser, _ := NewOperationDataFieldParser(args)

	t.Run("TransferNonHexArguments", func(t *testing.T) {
		t.Parallel()

		dataField := []byte("DCTTransfer@1234@011")
		res := parser.Parse(dataField, sender, receiver, 3)
		require.Equal(t, &ResponseParseData{
			Operation: operationTransfer,
		}, res)
	})

	t.Run("TransferNotEnoughArguments", func(t *testing.T) {
		t.Parallel()

		dataField := []byte("DCTTransfer@1234")
		res := parser.Parse(dataField, sender, receiver, 3)
		require.Equal(t, &ResponseParseData{
			Operation: "DCTTransfer",
		}, res)
	})

	t.Run("TransferEmptyArguments", func(t *testing.T) {
		t.Parallel()

		dataField := []byte("DCTTransfer@544f4b454e@")
		res := parser.Parse(dataField, sender, receiver, 3)
		require.Equal(t, &ResponseParseData{
			Operation: "DCTTransfer",
			Tokens:    []string{"TOKEN"},
			DCTValues: []string{"0"},
		}, res)
	})

	t.Run("TransferWithSCCall", func(t *testing.T) {
		t.Parallel()

		dataField := []byte("DCTTransfer@544f4b454e@01@63616c6c4d65")
		res := parser.Parse(dataField, sender, receiverSC, 3)
		require.Equal(t, &ResponseParseData{
			Operation:        "DCTTransfer",
			Function:         "",
			DCTValues:        []string{"1"},
			Tokens:           []string{"TOKEN"},
			Receivers:        [][]uint8(nil),
			ReceiversShardID: []uint32(nil),
			IsRelayed:        false,
		}, res)
	})

	t.Run("TransferNonAsciiStringToken", func(t *testing.T) {
		dataField := []byte("DCTTransfer@055de6a779bbac0000@01")
		res := parser.Parse(dataField, sender, receiverSC, 3)
		require.Equal(t, &ResponseParseData{
			Operation: "DCTTransfer",
		}, res)
	})
}
