package switchpayloadtype

import (
	"context"
	"fmt"
	"strconv"

	"stream-source-tester/internal/config"
	"stream-source-tester/internal/model"
	"stream-source-tester/internal/mutation"
)

func init() {
	if err := mutation.Register("switch-payloadtype", func() mutation.Mutator { return &Mutator{} }); err != nil {
		panic(err)
	}
}

// Mutator implements mid-stream Payload Type switching.
type Mutator struct{}

func (m *Mutator) Apply(ctx context.Context, bundle *model.SessionBundle, cfg config.MutationConfig) (*model.SessionBundle, error) {
	_ = ctx
	if bundle.Metadata == nil {
		bundle.Metadata = map[string]string{}
	}

	if len(bundle.Streams) == 0 {
		bundle.Metadata["mutation.switch-payloadtype"] = "skipped (no streams)"
		return bundle, nil
	}

	// Parse switch.at - sequence number or timestamp trigger
	switchAt := 100 // default: switch at packet 100
	if raw := cfg.Options["pt.switch.at"]; raw != "" {
		if val, err := strconv.Atoi(raw); err == nil {
			switchAt = val
		}
	}

	// Parse switch.to - new payload type
	newPT := uint8(99) // default
	if raw := cfg.Options["pt.switch.to"]; raw != "" {
		if val, err := strconv.ParseUint(raw, 10, 8); err == nil {
			newPT = uint8(val)
		}
	}

	// Parse switch.count - number of switches
	switchCount := 1
	if raw := cfg.Options["pt.switch.count"]; raw != "" {
		if val, err := strconv.Atoi(raw); err == nil && val > 0 {
			switchCount = val
		}
	}

	// Parse switch.mode
	switchMode := "immediate"
	if raw := cfg.Options["pt.switch.mode"]; raw != "" {
		switchMode = raw
	}

	// Track initial PT for history
	initialPT := bundle.Streams[0].PayloadType

	switch switchMode {
	case "immediate":
		switchCountLeft := switchCount
		nextSwitchAt := switchAt
		currentPT := initialPT

		for i := range bundle.Timeline {
			if i+1 >= nextSwitchAt && switchCountLeft > 0 {
				currentPT = newPT
				switchCountLeft--
				nextSwitchAt = switchAt + (switchCount-switchCountLeft)*(switchAt+1)
			}
			bundle.Timeline[i].Metadata = addPTOverride(bundle.Timeline[i].Metadata, currentPT)
		}

	case "alternate":
		// Switch between old and new PT every N packets
		alternateInterval := 30
		if raw := cfg.Options["pt.switch.alternate-interval"]; raw != "" {
			if val, err := strconv.Atoi(raw); err == nil && val > 0 {
				alternateInterval = val
			}
		}

		useNewPT := false
		for i := range bundle.Timeline {
			if i+1 >= switchAt {
				if i > 0 && (i-switchAt+1)%alternateInterval == 0 {
					useNewPT = !useNewPT
				}
			}
			pt := initialPT
			if useNewPT {
				pt = newPT
			}
			bundle.Timeline[i].Metadata = addPTOverride(bundle.Timeline[i].Metadata, pt)
		}

	default:
		// Default to immediate
		switchCountLeft := switchCount
		nextSwitchAt := switchAt
		for i := range bundle.Timeline {
			if i+1 >= nextSwitchAt && switchCountLeft > 0 {
				bundle.Timeline[i].Metadata = addPTOverride(bundle.Timeline[i].Metadata, newPT)
				switchCountLeft--
				nextSwitchAt = switchAt + (switchCount-switchCountLeft)*(switchAt+1)
			}
		}
	}

	bundle.Metadata["mutation.switch-payloadtype"] = fmt.Sprintf("mode=%s,at=%d,to=%d,count=%d", switchMode, switchAt, newPT, switchCount)
	bundle.Metadata["pt.history"] = fmt.Sprintf("initial=%d,current=%d", initialPT, newPT)

	return bundle, nil
}

func addPTOverride(metadata map[string]string, pt uint8) map[string]string {
	if metadata == nil {
		metadata = make(map[string]string)
	}
	metadata["override.pt"] = strconv.Itoa(int(pt))
	return metadata
}
