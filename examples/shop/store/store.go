// Package store saves orders to a file.
package store

import (
	"encoding/json"
	"os"
)

// Order is a saved order.
type Order struct {
	ID    string `json:"id"`
	Cents int    `json:"cents"`
}

// Save writes orders to path as JSON.
func Save(path string, orders []Order) error {
	data, err := json.Marshal(orders)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads orders written by Save.
func Load(path string) ([]Order, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var orders []Order
	err = json.Unmarshal(data, &orders)
	return orders, err
}
