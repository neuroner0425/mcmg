package mcservice

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ExperimentInfo defines metadata for standard built-in Minecraft experiments.
type ExperimentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// DefaultExperiments lists the official experimental gameplay packs.
var DefaultExperiments = []ExperimentInfo{
	{
		ID:          "trade_rebalance",
		Name:        "주민 거래 밸런스 조정 (Trade Rebalance)",
		Description: "바이옴별 주민 직업 및 마법이 부여된 책 거래 메커니즘을 개편합니다.",
	},
	{
		ID:          "minecart_improvements",
		Name:        "마인카트 개선 (Minecart Improvements)",
		Description: "더 빠르고 부드러운 카트 최고 속도 및 주행 물리엔진을 적용합니다.",
	},
	{
		ID:          "redstone_experiments",
		Name:        "레드스톤 실험 (Redstone Experiments)",
		Description: "레드스톤 전선과 신호 업데이트 일관성 및 퍼포먼스를 대폭 개선합니다.",
	},
}

// DatapackStatusResponse returns current datapack state.
type DatapackStatusResponse struct {
	EnabledPacks  []string         `json:"enabled_packs"`
	DisabledPacks []string         `json:"disabled_packs"`
	Experiments   []ExperimentInfo `json:"experiments"`
	RconOutput    string           `json:"rcon_output,omitempty"`
}

// DatapackManager handles reading and modifying datapack configuration across level.dat, server.properties and RCON.
type DatapackManager struct {
	serverDir string
}

// NewDatapackManager creates a DatapackManager instance.
func NewDatapackManager(serverDir string) *DatapackManager {
	return &DatapackManager{serverDir: serverDir}
}

// GetStatus inspects level.dat, server.properties and optional RCON to retrieve current pack status.
func (dm *DatapackManager) GetStatus(rconClient RCONClient) (*DatapackStatusResponse, error) {
	resp := &DatapackStatusResponse{
		EnabledPacks:  make([]string, 0),
		DisabledPacks: make([]string, 0),
	}

	// 1. Inspect level.dat if present
	levelDatPath := filepath.Join(dm.serverDir, "world", "level.dat")
	if root, err := ReadLevelDat(levelDatPath); err == nil {
		if dataTag := root.FindChild("Data"); dataTag != nil {
			if dpTag := dataTag.FindChild("DataPacks"); dpTag != nil {
				if enTag := dpTag.FindChild("Enabled"); enTag != nil {
					for _, item := range enTag.ListItems {
						if s, ok := item.Value.(string); ok {
							resp.EnabledPacks = append(resp.EnabledPacks, s)
						}
					}
				}
				if disTag := dpTag.FindChild("Disabled"); disTag != nil {
					for _, item := range disTag.ListItems {
						if s, ok := item.Value.(string); ok {
							resp.DisabledPacks = append(resp.DisabledPacks, s)
						}
					}
				}
			}
		}
	} else {
		// Fallback to server.properties initial-enabled-packs
		propsPath := filepath.Join(dm.serverDir, "server.properties")
		if data, err := os.ReadFile(propsPath); err == nil {
			content := string(data)
			re := regexp.MustCompile(`(?m)^\s*initial-enabled-packs\s*=\s*(.*)$`)
			if m := re.FindStringSubmatch(content); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
				for _, p := range strings.Split(m[1], ",") {
					if clean := strings.TrimSpace(p); clean != "" {
						resp.EnabledPacks = append(resp.EnabledPacks, clean)
					}
				}
			}
		}
	}

	// Map experiment enabled state
	enabledSet := make(map[string]bool)
	for _, p := range resp.EnabledPacks {
		enabledSet[p] = true
	}

	experiments := make([]ExperimentInfo, len(DefaultExperiments))
	for i, exp := range DefaultExperiments {
		experiments[i] = exp
		if enabledSet[exp.ID] {
			experiments[i].Enabled = true
		}
	}
	resp.Experiments = experiments

	// Optional RCON datapack list
	if rconClient != nil {
		if out, err := rconClient.Execute("datapack list"); err == nil {
			resp.RconOutput = out
		}
	}

	return resp, nil
}

// ApplyExperiments applies selected experiment IDs to level.dat, server.properties and active server via RCON.
func (dm *DatapackManager) ApplyExperiments(selectedExperiments []string, rconClient RCONClient) error {
	// 1. Update server.properties initial-enabled-packs
	if err := dm.updateServerProperties(selectedExperiments); err != nil {
		return fmt.Errorf("failed to update server.properties: %w", err)
	}

	// 2. Update level.dat if exists
	levelDatPath := filepath.Join(dm.serverDir, "world", "level.dat")
	if _, err := os.Stat(levelDatPath); err == nil {
		if err := dm.updateLevelDat(levelDatPath, selectedExperiments); err != nil {
			return fmt.Errorf("failed to update level.dat: %w", err)
		}
	}

	// 3. If server is running and RCON is available, execute live datapack commands
	if rconClient != nil {
		for _, id := range selectedExperiments {
			_, _ = rconClient.Execute(fmt.Sprintf("datapack enable \"%s\"", id))
		}
		// Also disable unselected experiments if they were active
		for _, exp := range DefaultExperiments {
			isSelected := false
			for _, s := range selectedExperiments {
				if s == exp.ID {
					isSelected = true
					break
				}
			}
			if !isSelected {
				_, _ = rconClient.Execute(fmt.Sprintf("datapack disable \"%s\"", exp.ID))
			}
		}
		// Refresh datapacks
		_, _ = rconClient.Execute("datapack list")
	}

	return nil
}

// updateServerProperties ensures initial-enabled-packs contains selected experiments.
func (dm *DatapackManager) updateServerProperties(selectedExperiments []string) error {
	propsPath := filepath.Join(dm.serverDir, "server.properties")
	data, err := os.ReadFile(propsPath)
	if err != nil {
		return err
	}

	content := string(data)
	basePacks := []string{"vanilla", "paper"}
	packSet := make(map[string]bool)
	finalPacks := make([]string, 0)

	for _, b := range basePacks {
		packSet[b] = true
		finalPacks = append(finalPacks, b)
	}

	for _, exp := range selectedExperiments {
		clean := strings.TrimSpace(exp)
		if clean != "" && !packSet[clean] {
			packSet[clean] = true
			finalPacks = append(finalPacks, clean)
		}
	}

	newEnabledLine := fmt.Sprintf("initial-enabled-packs=%s", strings.Join(finalPacks, ","))
	reEnabled := regexp.MustCompile(`(?m)^\s*initial-enabled-packs\s*=.*$`)
	if reEnabled.MatchString(content) {
		content = reEnabled.ReplaceAllString(content, newEnabledLine)
	} else {
		content = content + "\n" + newEnabledLine
	}

	reDisabled := regexp.MustCompile(`(?m)^\s*initial-disabled-packs\s*=.*$`)
	if reDisabled.MatchString(content) {
		content = reDisabled.ReplaceAllString(content, "initial-disabled-packs=")
	} else {
		content = content + "\ninitial-disabled-packs="
	}

	return os.WriteFile(propsPath, []byte(content), 0644)
}

// updateLevelDat updates DataPacks (Enabled/Disabled) and enabled_features tags in level.dat.
func (dm *DatapackManager) updateLevelDat(levelDatPath string, selectedExperiments []string) error {
	root, err := ReadLevelDat(levelDatPath)
	if err != nil {
		return err
	}

	dataTag := root.FindChild("Data")
	if dataTag == nil {
		return fmt.Errorf("Data tag not found in level.dat")
	}

	dpTag := dataTag.FindChild("DataPacks")
	if dpTag == nil {
		dpTag = &NBTTag{Type: TagCompound, Name: "DataPacks", Children: make([]*NBTTag, 0)}
		dataTag.SetChild(dpTag)
	}

	// 1. Manage Enabled TagList
	enTag := dpTag.FindChild("Enabled")
	if enTag == nil {
		enTag = &NBTTag{Type: TagList, Name: "Enabled", ListType: TagString, ListItems: make([]*NBTTag, 0)}
		dpTag.SetChild(enTag)
	}

	enabledSet := make(map[string]bool)
	newEnabledItems := make([]*NBTTag, 0)

	// Keep existing non-experiment packs (like vanilla, paper, plugins)
	for _, item := range enTag.ListItems {
		name := item.Value.(string)
		isKnownExp := false
		for _, exp := range DefaultExperiments {
			if exp.ID == name {
				isKnownExp = true
				break
			}
		}
		if !isKnownExp {
			enabledSet[name] = true
			newEnabledItems = append(newEnabledItems, item)
		}
	}

	// Always ensure vanilla and paper
	if !enabledSet["vanilla"] {
		enabledSet["vanilla"] = true
		newEnabledItems = append(newEnabledItems, &NBTTag{Type: TagString, Value: "vanilla"})
	}

	// Append selected experiments
	for _, id := range selectedExperiments {
		clean := strings.TrimSpace(id)
		if clean != "" && !enabledSet[clean] {
			enabledSet[clean] = true
			newEnabledItems = append(newEnabledItems, &NBTTag{Type: TagString, Value: clean})
		}
	}
	enTag.ListItems = newEnabledItems

	// 2. Manage Disabled TagList (remove selected, add unselected)
	disTag := dpTag.FindChild("Disabled")
	if disTag == nil {
		disTag = &NBTTag{Type: TagList, Name: "Disabled", ListType: TagString, ListItems: make([]*NBTTag, 0)}
		dpTag.SetChild(disTag)
	}

	newDisabledItems := make([]*NBTTag, 0)
	disabledSet := make(map[string]bool)

	// Retain unselected known experiments in Disabled list so server recognizes them
	for _, exp := range DefaultExperiments {
		if !enabledSet[exp.ID] {
			disabledSet[exp.ID] = true
			newDisabledItems = append(newDisabledItems, &NBTTag{Type: TagString, Value: exp.ID})
		}
	}
	disTag.ListItems = newDisabledItems

	// 3. Manage enabled_features tag in Data tag (Critical for Minecraft 1.20+ to permit experimental features)
	featTag := dataTag.FindChild("enabled_features")
	if featTag == nil {
		featTag = &NBTTag{Type: TagList, Name: "enabled_features", ListType: TagString, ListItems: make([]*NBTTag, 0)}
		dataTag.SetChild(featTag)
	}

	featItems := make([]*NBTTag, 0)
	featSet := make(map[string]bool)

	// vanilla feature
	featSet["minecraft:vanilla"] = true
	featItems = append(featItems, &NBTTag{Type: TagString, Value: "minecraft:vanilla"})

	for _, id := range selectedExperiments {
		clean := strings.TrimSpace(id)
		featureKey := "minecraft:" + clean
		if !featSet[featureKey] {
			featSet[featureKey] = true
			featItems = append(featItems, &NBTTag{Type: TagString, Value: featureKey})
		}
	}
	featTag.ListItems = featItems

	return WriteLevelDat(levelDatPath, root)
}
