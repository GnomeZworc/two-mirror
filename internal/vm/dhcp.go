package vm

import (
	"git.g3e.fr/syonad/two/internal/dhcpbackend"
)

type subnetReservations struct {
	subnet       dhcpbackend.Subnet
	reservations []dhcpbackend.Reservation
}
