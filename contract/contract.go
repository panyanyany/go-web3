package contract

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/panyanyany/go-web3"
	"github.com/panyanyany/go-web3/abi"
	"github.com/panyanyany/go-web3/jsonrpc"
)

// Contract is an Ethereum contract
type Contract struct {
	Address  web3.Address
	From     *web3.Address
	Abi      *abi.ABI
	Provider jsonrpc.IEth
}

// DeployContract deploys a contract
func DeployContract(provider jsonrpc.IEth, from web3.Address, abi *abi.ABI, bin []byte, args ...interface{}) *Txn {
	return &Txn{
		from:     from,
		provider: provider,
		method:   abi.Constructor,
		args:     args,
		bin:      bin,
	}
}

// NewContract creates a new contract instance
func NewContract(addr web3.Address, abi *abi.ABI, provider jsonrpc.IEth) *Contract {
	return &Contract{
		Address:  addr,
		Abi:      abi,
		Provider: provider,
	}
}

// ABI returns the Abi of the contract
func (c *Contract) ABI() *abi.ABI {
	return c.Abi
}

// Addr returns the address of the contract
func (c *Contract) Addr() web3.Address {
	return c.Address
}

// SetFrom sets the origin of the calls
func (c *Contract) SetFrom(addr web3.Address) {
	c.From = &addr
}

// EstimateGas estimates the gas for a contract call
func (c *Contract) EstimateGas(method string, args ...interface{}) (uint64, error) {
	return c.Txn(method, args).EstimateGas()
}

// Call calls a method in the contract
func (c *Contract) Call(method string, block web3.BlockNumber, args ...interface{}) (map[string]interface{}, error) {
	m, ok := c.Abi.Methods[method]
	if !ok {
		return nil, fmt.Errorf("method %s not found in Contract.Abi.Methods[method]", method)
	}

	// Encode input
	data, err := abi.Encode(args, m.Inputs)
	if err != nil {
		err = fmt.Errorf("aib.Encode(): %w", err)
		return nil, err
	}
	data = append(m.ID(), data...)

	// Call function
	msg := &web3.CallMsg{
		To:   &c.Address,
		Data: data,
	}
	if c.From != nil {
		msg.From = *c.From
	}

	rawStr, err := c.Provider.Call(msg, block)
	if err != nil {
		err = fmt.Errorf("Contract.Provider.Call(): %w", err)
		return nil, err
	}

	// Decode output
	raw, err := hex.DecodeString(rawStr[2:])
	if err != nil {
		err = fmt.Errorf("hex.DecodeString: %w", err)
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	respInterface, err := abi.Decode(m.Outputs, raw)
	if err != nil {
		err = fmt.Errorf("Abi.Decode: %w", err)
		return nil, err
	}

	resp := respInterface.(map[string]interface{})
	return resp, nil
}

// Txn creates a new transaction object
func (c *Contract) Txn(method string, args ...interface{}) *Txn {
	m, ok := c.Abi.Methods[method]
	if !ok {
		// TODO, return error
		panic(fmt.Errorf("method %s not found", method))
	}

	return &Txn{
		from:     *c.From,
		addr:     &c.Address,
		provider: c.Provider,
		method:   m,
		args:     args,
	}
}

// Txn is a transaction object
type Txn struct {
	from     web3.Address
	addr     *web3.Address
	provider jsonrpc.IEth
	method   *abi.Method
	args     []interface{}
	data     []byte
	bin      []byte
	gasLimit uint64
	gasPrice uint64
	value    *big.Int
	hash     web3.Hash
	receipt  *web3.Receipt
}

func (t *Txn) isContractDeployment() bool {
	return t.bin != nil
}

// AddArgs is used to set the arguments of the transaction
func (t *Txn) AddArgs(args ...interface{}) *Txn {
	t.args = args
	return t
}

// SetValue sets the value for the txn
func (t *Txn) SetValue(v *big.Int) *Txn {
	t.value = new(big.Int).Set(v)
	return t
}

// EstimateGas estimates the gas for the call
func (t *Txn) EstimateGas() (uint64, error) {
	if err := t.Validate(); err != nil {
		return 0, err
	}
	return t.estimateGas()
}

func (t *Txn) estimateGas() (uint64, error) {
	if t.isContractDeployment() {
		return t.provider.EstimateGasContract(t.data)
	}

	msg := &web3.CallMsg{
		From:  t.from,
		To:    t.addr,
		Data:  t.data,
		Value: t.value,
	}
	return t.provider.EstimateGas(msg)
}

// DoAndWait is a blocking query that combines
// both Do and Wait functions
func (t *Txn) DoAndWait() error {
	if err := t.Do(); err != nil {
		return err
	}
	if err := t.Wait(); err != nil {
		return err
	}
	return nil
}

// Do sends the transaction to the network
func (t *Txn) Do() error {
	err := t.Validate()
	if err != nil {
		return err
	}

	// estimate gas price
	if t.gasPrice == 0 {
		t.gasPrice, err = t.provider.GasPrice()
		if err != nil {
			return err
		}
	}
	// estimate gas limit
	if t.gasLimit == 0 {
		t.gasLimit, err = t.estimateGas()
		if err != nil {
			return err
		}
	}

	// send transaction
	txn := &web3.Transaction{
		From:     t.from,
		Input:    t.data,
		GasPrice: t.gasPrice,
		Gas:      t.gasLimit,
		Value:    t.value,
	}
	if t.addr != nil {
		txn.To = t.addr
	}
	t.hash, err = t.provider.SendTransaction(txn)
	if err != nil {
		return err
	}
	return nil
}

// Validate validates the arguments of the transaction
func (t *Txn) Validate() error {
	if t.data != nil {
		// Already validated
		return nil
	}
	if t.isContractDeployment() {
		t.data = append(t.data, t.bin...)
	}
	if t.method != nil {
		data, err := abi.Encode(t.args, t.method.Inputs)
		if err != nil {
			return fmt.Errorf("failed to encode arguments: %v", err)
		}
		if !t.isContractDeployment() {
			t.data = append(t.method.ID(), data...)
		} else {
			t.data = append(t.data, data...)
		}
	}
	return nil
}

// SetGasPrice sets the gas price of the transaction
func (t *Txn) SetGasPrice(gasPrice uint64) *Txn {
	t.gasPrice = gasPrice
	return t
}

// SetGasLimit sets the gas limit of the transaction
func (t *Txn) SetGasLimit(gasLimit uint64) *Txn {
	t.gasLimit = gasLimit
	return t
}

// Wait waits till the transaction is mined
func (t *Txn) Wait() error {
	if (t.hash == web3.Hash{}) {
		panic("transaction not executed")
	}

	var err error
	for {
		t.receipt, err = t.provider.GetTransactionReceipt(t.hash)
		if err != nil {
			if err.Error() != "not found" {
				return err
			}
		}
		if t.receipt != nil {
			break
		}
	}
	return nil
}

// Receipt returns the receipt of the transaction after wait
func (t *Txn) Receipt() *web3.Receipt {
	return t.receipt
}

// Event is a solidity event
type Event struct {
	event *abi.Event
}

// Encode encodes an event
func (e *Event) Encode() web3.Hash {
	return e.event.ID()
}

// ParseLog parses a log
func (e *Event) ParseLog(log *web3.Log) (map[string]interface{}, error) {
	return abi.ParseLog(e.event.Inputs, log)
}

// Event returns a specific event
func (c *Contract) Event(name string) (*Event, bool) {
	event, ok := c.Abi.Events[name]
	if !ok {
		return nil, false
	}
	return &Event{event}, true
}
