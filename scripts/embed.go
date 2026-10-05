package scripts

import _ "embed"

//go:embed deploy.sh
var Deploy []byte

//go:embed bootstrap_kvm.sh
var BootstrapKVM []byte
