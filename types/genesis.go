package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	tmproto "github.com/tendermint/tendermint/proto/types"

	"github.com/pkg/errors"

	"github.com/tendermint/tendermint/crypto"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmtime "github.com/tendermint/tendermint/types/time"
)

const (
	// MaxChainIDLen is a maximum length of the chain ID.
	MaxChainIDLen = 50
)

//------------------------------------------------------------
// core types for a genesis definition
// NOTE: any changes to the genesis definition should
// be reflected in the documentation:
// docs/tendermint-core/using-tendermint.md

// GenesisValidator is an initial validator.
type GenesisValidator struct {
	Address Address       `json:"address"`
	PubKey  crypto.PubKey `json:"pub_key"`
	Power   int64         `json:"power"`
	Name    string        `json:"name"`
}

type VoterParams struct {
	VoterElectionThreshold          int32 `json:"voter_election_threshold"`
	MaxTolerableByzantinePercentage int32 `json:"max_tolerable_byzantine_percentage"`

	// As a unit of precision, if it is 1, it is 0.9, and if it is 2, it is 0.99.
	// The default is 5, with a precision of 0.99999.
	ElectionPrecision int32 `json:"election_precision"`
}

// GenesisDoc defines the initial conditions for a tendermint blockchain, in particular its validator set.
type GenesisDoc struct {
	GenesisTime     time.Time          `json:"genesis_time"`
	ChainID         string             `json:"chain_id"`
	ConsensusParams *ConsensusParams   `json:"consensus_params,omitempty"`
	Validators      []GenesisValidator `json:"validators,omitempty"`
	VoterParams     *VoterParams       `json:"voter_params,omitempty"`
	AppHash         tmbytes.HexBytes   `json:"app_hash"`
	AppState        json.RawMessage    `json:"app_state,omitempty"`
}

// SaveAs is a utility method for saving GenensisDoc as a JSON file.
func (genDoc *GenesisDoc) SaveAs(file string) error {
	genDocBytes, err := cdc.MarshalJSONIndent(genDoc, "", "  ")
	if err != nil {
		return err
	}
	return tmos.WriteFile(file, genDocBytes, 0644)
}

// ValidatorHash returns the hash of the validator set contained in the GenesisDoc
func (genDoc *GenesisDoc) ValidatorHash() []byte {
	vals := make([]*Validator, len(genDoc.Validators))
	for i, v := range genDoc.Validators {
		vals[i] = NewValidator(v.PubKey, v.Power)
	}
	vset := NewValidatorSet(vals)
	return vset.Hash()
}

// ValidateAndComplete checks that all necessary fields are present
// and fills in defaults for optional fields left empty
func (genDoc *GenesisDoc) ValidateAndComplete() error {
	if genDoc.ChainID == "" {
		return errors.New("genesis doc must include non-empty chain_id")
	}
	if len(genDoc.ChainID) > MaxChainIDLen {
		return errors.Errorf("chain_id in genesis doc is too long (max: %d)", MaxChainIDLen)
	}

	if genDoc.ConsensusParams == nil {
		genDoc.ConsensusParams = DefaultConsensusParams()
	} else if err := genDoc.ConsensusParams.Validate(); err != nil {
		return err
	}

	if genDoc.VoterParams == nil {
		genDoc.VoterParams = DefaultVoterParams()
	} else if err := genDoc.VoterParams.Validate(); err != nil {
		return err
	}

	for i, v := range genDoc.Validators {
		if v.Power == 0 {
			return errors.Errorf("the genesis file cannot contain validators with no voting power: %v", v)
		}
		if len(v.Address) > 0 && !bytes.Equal(v.PubKey.Address(), v.Address) {
			return errors.Errorf("incorrect address for validator %v in the genesis file, should be %v", v, v.PubKey.Address())
		}
		if len(v.Address) == 0 {
			genDoc.Validators[i].Address = v.PubKey.Address()
		}
	}

	if genDoc.GenesisTime.IsZero() {
		genDoc.GenesisTime = tmtime.Now()
	}

	return nil
}

// Hash returns the hash of the GenesisDoc
func (genDoc *GenesisDoc) Hash() []byte {
	return cdcEncode(genDoc)
}

//------------------------------------------------------------
// Make genesis state from file

// GenesisDocFromJSON unmarshalls JSON data into a GenesisDoc.
func GenesisDocFromJSON(jsonBlob []byte) (*GenesisDoc, error) {
	genDoc := GenesisDoc{}
	err := cdc.UnmarshalJSON(jsonBlob, &genDoc)
	if err != nil {
		return nil, err
	}

	if err := genDoc.ValidateAndComplete(); err != nil {
		return nil, err
	}

	return &genDoc, err
}

// GenesisDocFromFile reads JSON data from a file and unmarshalls it into a GenesisDoc.
func GenesisDocFromFile(genDocFile string) (*GenesisDoc, error) {
	jsonBlob, err := ioutil.ReadFile(genDocFile)
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't read GenesisDoc file")
	}
	genDoc, err := GenesisDocFromJSON(jsonBlob)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error reading GenesisDoc at %v", genDocFile))
	}
	return genDoc, nil
}

func (vp *VoterParams) Validate() error {
	if vp.VoterElectionThreshold < 0 {
		return errors.Errorf("VoterElectionThreshold must be greater than or equal to 0. Got %d",
			vp.VoterElectionThreshold)
	}
	if vp.MaxTolerableByzantinePercentage <= 0 || vp.MaxTolerableByzantinePercentage >= 34 {
		return errors.Errorf("MaxTolerableByzantinePercentage must be in between 1 and 33. Got %d",
			vp.MaxTolerableByzantinePercentage)
	}
	if vp.ElectionPrecision <= 1 || vp.ElectionPrecision > 15 {
		return errors.Errorf("ElectionPrecision must be in 2~15(including). Got %d", vp.ElectionPrecision)
	}
	return nil
}

func (vp *VoterParams) ToProto() *tmproto.VoterParams {
	if vp == nil {
		return nil
	}

	return &tmproto.VoterParams{
		VoterElectionThreshold:          vp.VoterElectionThreshold,
		MaxTolerableByzantinePercentage: vp.MaxTolerableByzantinePercentage,
		ElectionPrecision:               vp.ElectionPrecision,
	}
}

func (vp *VoterParams) FromProto(vpp *tmproto.VoterParams) {
	vp.VoterElectionThreshold = vpp.VoterElectionThreshold
	vp.MaxTolerableByzantinePercentage = vpp.MaxTolerableByzantinePercentage
	vp.ElectionPrecision = vpp.ElectionPrecision
}
