package dispatcher

import "github.com/dgraph-io/badger/v4"

type CreateSubnetCommand struct {
	Name      string
	VPC       string
	VxlanID   int
	LocalIP   string
	GatewayIP string
	CIDR      string
}

func (c CreateSubnetCommand) Execute(db *badger.DB) error {
	// TODO: brancher internal/subnet/create.go
	return nil
}

type DeleteSubnetCommand struct {
	Name string
}

func (c DeleteSubnetCommand) Execute(db *badger.DB) error {
	// TODO: brancher internal/subnet/delete.go
	return nil
}
