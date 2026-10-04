package main

import (
	"net"

	"localdrop/internal/relay"
	"localdrop/internal/server"
)

func prepareMediaRelay(request server.MediaRelayRequest) (*server.MediaRelayLease, error) {
	lease, err := relay.New(relay.Config{
		RelayIP: net.ParseIP(request.ServerIP),
		HostIP:  net.ParseIP(request.HostIP),
		PeerIP:  net.ParseIP(request.ViewerIP),
	})
	if err != nil {
		return nil, err
	}
	convert := func(config relay.ICEServer) []server.MediaICEServer {
		return []server.MediaICEServer{{
			URLs:       append([]string(nil), config.URLs...),
			Username:   config.Username,
			Credential: config.Credential,
		}}
	}
	return &server.MediaRelayLease{
		HostICE:   convert(lease.HostICE()),
		ViewerICE: convert(lease.PeerICE()),
		Revoke:    lease.Revoke,
		Close:     lease.Close,
	}, nil
}
