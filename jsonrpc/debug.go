package jsonrpc

import "github.com/panyanyany/go-web3"

type Debug struct {
	c *Client
}

type TransactionTrace struct {
	Gas         uint64
	ReturnValue string
	StructLogs  []*StructLogs
}

type StructLogs struct {
	Depth   int
	Gas     int
	GasCost int
	Op      string
	Pc      int
	Memory  []string
	Stack   []string
	Storage map[string]string
}

func (d *Debug) TraceTransaction(hash web3.Hash) (*TransactionTrace, error) {
	var res *TransactionTrace
	err := d.c.Call("debug_traceTransaction", &res, hash)
	return res, err
}
