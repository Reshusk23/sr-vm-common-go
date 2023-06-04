package builtInFunctions

import (
	"bytes"
	"fmt"
	"math/big"
	"sync"

	"github.com/Reshusk23/sr-me-core/core"
	"github.com/Reshusk23/sr-me-core/core/check"
	"github.com/Reshusk23/sr-me-core/data/dct"
	"github.com/Reshusk23/sr-me-core/data/vm"
	logger "github.com/Reshusk23/sr-me-logger"
	vmcommon "github.com/Reshusk23/sr-vm-common-go"
)

var (
	log         = logger.GetOrCreate("builtInFunctions")
	noncePrefix = []byte(core.ProtectedKeyPrefix + core.DCTNFTLatestNonceIdentifier)
)

type dctNFTCreate struct {
	baseAlwaysActiveHandler
	keyPrefix             []byte
	accounts              vmcommon.AccountsAdapter
	marshaller            vmcommon.Marshalizer
	globalSettingsHandler vmcommon.DCTGlobalSettingsHandler
	rolesHandler          vmcommon.DCTRoleHandler
	funcGasCost           uint64
	gasConfig             vmcommon.BaseOperationCost
	dctStorageHandler     vmcommon.DCTNFTStorageHandler
	enableEpochsHandler   vmcommon.EnableEpochsHandler
	mutExecution          sync.RWMutex
}

// NewDCTNFTCreateFunc returns the dct NFT create built-in function component
func NewDCTNFTCreateFunc(
	funcGasCost uint64,
	gasConfig vmcommon.BaseOperationCost,
	marshaller vmcommon.Marshalizer,
	globalSettingsHandler vmcommon.DCTGlobalSettingsHandler,
	rolesHandler vmcommon.DCTRoleHandler,
	dctStorageHandler vmcommon.DCTNFTStorageHandler,
	accounts vmcommon.AccountsAdapter,
	enableEpochsHandler vmcommon.EnableEpochsHandler,
) (*dctNFTCreate, error) {
	if check.IfNil(marshaller) {
		return nil, ErrNilMarshalizer
	}
	if check.IfNil(globalSettingsHandler) {
		return nil, ErrNilGlobalSettingsHandler
	}
	if check.IfNil(rolesHandler) {
		return nil, ErrNilRolesHandler
	}
	if check.IfNil(dctStorageHandler) {
		return nil, ErrNilDCTNFTStorageHandler
	}
	if check.IfNil(enableEpochsHandler) {
		return nil, ErrNilEnableEpochsHandler
	}
	if check.IfNil(accounts) {
		return nil, ErrNilAccountsAdapter
	}

	e := &dctNFTCreate{
		keyPrefix:             []byte(baseDCTKeyPrefix),
		marshaller:            marshaller,
		globalSettingsHandler: globalSettingsHandler,
		rolesHandler:          rolesHandler,
		funcGasCost:           funcGasCost,
		gasConfig:             gasConfig,
		dctStorageHandler:     dctStorageHandler,
		enableEpochsHandler:   enableEpochsHandler,
		mutExecution:          sync.RWMutex{},
		accounts:              accounts,
	}

	return e, nil
}

// SetNewGasConfig is called whenever gas cost is changed
func (e *dctNFTCreate) SetNewGasConfig(gasCost *vmcommon.GasCost) {
	if gasCost == nil {
		return
	}

	e.mutExecution.Lock()
	e.funcGasCost = gasCost.BuiltInCost.DCTNFTCreate
	e.gasConfig = gasCost.BaseOperationCost
	e.mutExecution.Unlock()
}

// ProcessBuiltinFunction resolves DCT NFT create function call
// Requires at least 7 arguments:
// arg0 - token identifier
// arg1 - initial quantity
// arg2 - NFT name
// arg3 - Royalties - max 10000
// arg4 - hash
// arg5 - attributes
// arg6+ - multiple entries of URI (minimum 1)
func (e *dctNFTCreate) ProcessBuiltinFunction(
	acntSnd, _ vmcommon.UserAccountHandler,
	vmInput *vmcommon.ContractCallInput,
) (*vmcommon.VMOutput, error) {
	e.mutExecution.RLock()
	defer e.mutExecution.RUnlock()

	err := checkDCTNFTCreateBurnAddInput(acntSnd, vmInput, e.funcGasCost)
	if err != nil {
		return nil, err
	}

	minNumOfArgs := 7
	if vmInput.CallType == vm.ExecOnDestByCaller {
		minNumOfArgs = 8
	}
	lenArgs := len(vmInput.Arguments)
	if lenArgs < minNumOfArgs {
		return nil, fmt.Errorf("%w, wrong number of arguments", ErrInvalidArguments)
	}

	accountWithRoles := acntSnd
	uris := vmInput.Arguments[6:]
	if vmInput.CallType == vm.ExecOnDestByCaller {
		scAddressWithRoles := vmInput.Arguments[lenArgs-1]
		uris = vmInput.Arguments[6 : lenArgs-1]

		if len(scAddressWithRoles) != len(vmInput.CallerAddr) {
			return nil, ErrInvalidAddressLength
		}
		if bytes.Equal(scAddressWithRoles, vmInput.CallerAddr) {
			return nil, ErrInvalidRcvAddr
		}

		accountWithRoles, err = e.getAccount(scAddressWithRoles)
		if err != nil {
			return nil, err
		}
	}

	tokenID := vmInput.Arguments[0]
	err = e.rolesHandler.CheckAllowedToExecute(accountWithRoles, vmInput.Arguments[0], []byte(core.DCTRoleNFTCreate))
	if err != nil {
		return nil, err
	}

	nonce, err := getLatestNonce(accountWithRoles, tokenID)
	if err != nil {
		return nil, err
	}

	totalLength := uint64(0)
	for _, arg := range vmInput.Arguments {
		totalLength += uint64(len(arg))
	}
	gasToUse := totalLength*e.gasConfig.StorePerByte + e.funcGasCost
	if vmInput.GasProvided < gasToUse {
		return nil, ErrNotEnoughGas
	}

	royalties := uint32(big.NewInt(0).SetBytes(vmInput.Arguments[3]).Uint64())
	if royalties > core.MaxRoyalty {
		return nil, fmt.Errorf("%w, invalid max royality value", ErrInvalidArguments)
	}

	dctTokenKey := append(e.keyPrefix, vmInput.Arguments[0]...)
	quantity := big.NewInt(0).SetBytes(vmInput.Arguments[1])
	if quantity.Cmp(zero) <= 0 {
		return nil, fmt.Errorf("%w, invalid quantity", ErrInvalidArguments)
	}
	if quantity.Cmp(big.NewInt(1)) > 0 {
		err = e.rolesHandler.CheckAllowedToExecute(accountWithRoles, vmInput.Arguments[0], []byte(core.DCTRoleNFTAddQuantity))
		if err != nil {
			return nil, err
		}
	}
	isValueLengthCheckFlagEnabled := e.enableEpochsHandler.IsValueLengthCheckFlagEnabled()
	if isValueLengthCheckFlagEnabled && len(vmInput.Arguments[1]) > maxLenForAddNFTQuantity {
		return nil, fmt.Errorf("%w max length for quantity in nft create is %d", ErrInvalidArguments, maxLenForAddNFTQuantity)
	}

	nextNonce := nonce + 1
	dctData := &dct.DCToken{
		Type:  uint32(core.NonFungible),
		Value: quantity,
		TokenMetaData: &dct.MetaData{
			Nonce:      nextNonce,
			Name:       vmInput.Arguments[2],
			Creator:    vmInput.CallerAddr,
			Royalties:  royalties,
			Hash:       vmInput.Arguments[4],
			Attributes: vmInput.Arguments[5],
			URIs:       uris,
		},
	}

	_, err = e.dctStorageHandler.SaveDCTNFTToken(accountWithRoles.AddressBytes(), accountWithRoles, dctTokenKey, nextNonce, dctData, true, vmInput.ReturnCallAfterError)
	if err != nil {
		return nil, err
	}
	err = e.dctStorageHandler.AddToLiquiditySystemAcc(dctTokenKey, nextNonce, quantity)
	if err != nil {
		return nil, err
	}

	err = saveLatestNonce(accountWithRoles, tokenID, nextNonce)
	if err != nil {
		return nil, err
	}

	if vmInput.CallType == vm.ExecOnDestByCaller {
		err = e.accounts.SaveAccount(accountWithRoles)
		if err != nil {
			return nil, err
		}
	}

	vmOutput := &vmcommon.VMOutput{
		ReturnCode:   vmcommon.Ok,
		GasRemaining: vmInput.GasProvided - gasToUse,
		ReturnData:   [][]byte{big.NewInt(0).SetUint64(nextNonce).Bytes()},
	}

	dctDataBytes, err := e.marshaller.Marshal(dctData)
	if err != nil {
		log.Warn("dctNFTCreate.ProcessBuiltinFunction: cannot marshall dct data for log", "error", err)
	}

	addDCTEntryInVMOutput(vmOutput, []byte(core.BuiltInFunctionDCTNFTCreate), vmInput.Arguments[0], nextNonce, quantity, vmInput.CallerAddr, dctDataBytes)

	return vmOutput, nil
}

func (e *dctNFTCreate) getAccount(address []byte) (vmcommon.UserAccountHandler, error) {
	account, err := e.accounts.LoadAccount(address)
	if err != nil {
		return nil, err
	}

	userAcc, ok := account.(vmcommon.UserAccountHandler)
	if !ok {
		return nil, ErrWrongTypeAssertion
	}

	return userAcc, nil
}

func getLatestNonce(acnt vmcommon.UserAccountHandler, tokenID []byte) (uint64, error) {
	nonceKey := getNonceKey(tokenID)
	nonceData, _, err := acnt.AccountDataHandler().RetrieveValue(nonceKey)
	if err != nil {
		return 0, err
	}

	if len(nonceData) == 0 {
		return 0, nil
	}

	return big.NewInt(0).SetBytes(nonceData).Uint64(), nil
}

func saveLatestNonce(acnt vmcommon.UserAccountHandler, tokenID []byte, nonce uint64) error {
	nonceKey := getNonceKey(tokenID)
	return acnt.AccountDataHandler().SaveKeyValue(nonceKey, big.NewInt(0).SetUint64(nonce).Bytes())
}

func computeDCTNFTTokenKey(dctTokenKey []byte, nonce uint64) []byte {
	return append(dctTokenKey, big.NewInt(0).SetUint64(nonce).Bytes()...)
}

func checkDCTNFTCreateBurnAddInput(
	account vmcommon.UserAccountHandler,
	vmInput *vmcommon.ContractCallInput,
	funcGasCost uint64,
) error {
	err := checkBasicDCTArguments(vmInput)
	if err != nil {
		return err
	}
	if !bytes.Equal(vmInput.CallerAddr, vmInput.RecipientAddr) {
		return ErrInvalidRcvAddr
	}
	if check.IfNil(account) && vmInput.CallType != vm.ExecOnDestByCaller {
		return ErrNilUserAccount
	}
	if vmInput.GasProvided < funcGasCost {
		return ErrNotEnoughGas
	}
	return nil
}

func getNonceKey(tokenID []byte) []byte {
	return append(noncePrefix, tokenID...)
}

// IsInterfaceNil returns true if underlying object in nil
func (e *dctNFTCreate) IsInterfaceNil() bool {
	return e == nil
}
