package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

// StickerItem is a single entry in the assistant's sticker catalog.
type StickerItem struct {
	Key         string `json:"key"`
	FileID      string `json:"file_id"`
	Description string `json:"description,omitempty"`
}

// StickerCatalog maps human-readable keys to Telegram sticker file_ids.
type StickerCatalog struct {
	Stickers []StickerItem `json:"stickers"`
}

// LoadStickerCatalog reads the catalog from a JSON file.
func LoadStickerCatalog(path string) (*StickerCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sticker catalog: %w", err)
	}

	var c StickerCatalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse sticker catalog: %w", err)
	}

	return &c, nil
}

// Lookup returns the file_id for the given key.
func (c *StickerCatalog) Lookup(key string) (string, bool) {
	for _, s := range c.Stickers {
		if s.Key == key {
			return s.FileID, true
		}
	}
	return "", false
}

// Keys returns all sticker keys in catalog order.
func (c *StickerCatalog) Keys() []string {
	keys := make([]string, 0, len(c.Stickers))
	for _, s := range c.Stickers {
		keys = append(keys, s.Key)
	}
	return keys
}
