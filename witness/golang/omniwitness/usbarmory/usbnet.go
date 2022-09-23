// https://github.com/usbarmory/tamago-example
//
// Copyright (c) F-Secure Corporation
// https://foundry.f-secure.com
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build usbarmory
// +build usbarmory

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/third_party/dhcp"
	"github.com/miekg/dns"
	usbnet "github.com/usbarmory/imx-usbnet"
	"github.com/usbarmory/tamago/soc/nxp/imx6ul"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

const (
	deviceMAC       = "1a:55:89:a2:69:41"
	hostMAC         = "1a:55:89:a2:69:42"
	defaultResolver = "8.8.8.8:53"
	dhcpServerPort  = 67
	dhcpClientPort  = 68
	nicID           = 1
)

var (
	iface    *usbnet.Interface
	resolver string = defaultResolver
	netReady sync.WaitGroup
)

// initNetworking initialises the USB network stack.
// ip should be a string representation of the IPv4 address to assign
// to the interface, or the empty string in which DHCP4 will be used to
// attempt to auto-configure the interface.
func initNetworking(ip string) error {
	var err error

	iface, err = usbnet.Init(ip, deviceMAC, hostMAC, 1)

	if err != nil {
		return fmt.Errorf("could not initialize USB networking, %v", err)
	}

	iface.EnableICMP()

	imx6ul.USB1.Init()
	imx6ul.USB1.DeviceMode()
	imx6ul.USB1.Reset()

	if ip == "" {
		netReady.Add(1)
		glog.Info("No IP address specified for NIC, using DHCP4")
		go runDHCP(context.Background())
	}
	return nil
}

func runNetworking() error {
	// never returns
	imx6ul.USB1.Start(iface.Device())

	return nil
}

// awaitNetwork will block until the network stack is ready to be used by the
// application.
// Currently this will only block when we're using DHCP to configure the
// interface, but no lease has yet been acquired.
func awaitNetwork() {
	// TODO(al): Replace this construction with a callback+context.
	netReady.Wait()
}

// runDHCP starts the dhcp client.
// Will not return until ctx is Done.
func runDHCP(ctx context.Context) {
	acquired := func(oldAddr, newAddr tcpip.AddressWithPrefix, cfg dhcp.Config) {
		glog.V(1).Infof("DHCPC: lease update - old: %v, new: %v", oldAddr.String(), newAddr.String())
		if oldAddr.Address == newAddr.Address && oldAddr.PrefixLen == newAddr.PrefixLen {
			glog.V(1).Infof("DHCPC: existing lease on %v renewed", newAddr.String())
			return
		}
		newProtoAddr := tcpip.ProtocolAddress{
			Protocol:          ipv4.ProtocolNumber,
			AddressWithPrefix: newAddr,
		}
		if !oldAddr.Address.Unspecified() {
			glog.V(1).Infof("DHCPC: Releasing %v", oldAddr.String())
			if err := iface.Stack.RemoveAddress(nicID, oldAddr.Address); err != nil {
				glog.Warningf("Failed to remove expired address from stack: %v", err)
			} else {
				netReady.Add(1)
			}
		}

		if !newAddr.Address.Unspecified() {
			glog.Infof("DHCPC: Acquired %v", newAddr.String())
			if err := iface.Stack.AddProtocolAddress(nicID, newProtoAddr, stack.AddressProperties{PEB: stack.FirstPrimaryEndpoint}); err != nil {
				glog.Warningf("Failed to add newly acquired address to stack: %v", err)
			} else {
				defer netReady.Done()
			}
			if len(cfg.DNS) > 0 {
				resolver = fmt.Sprintf("%s:53", cfg.DNS[0].String())
				glog.Infof("DHCPC: Using DNS server %v", resolver)
			}
		} else {
			glog.Warning("DHCPC: no address acquired")
		}
	}
	c := dhcp.NewClient(iface.Stack, nicID, iface.Link.LinkAddress(), 30*time.Second, time.Second, time.Second, acquired)
	c.Run(ctx)
}

func resolve(s string) (r *dns.Msg, rtt time.Duration, err error) {
	if s[len(s)-1:] != "." {
		s += "."
	}
	msg := new(dns.Msg)
	msg.Id = dns.Id()
	msg.RecursionDesired = true

	msg.Question = []dns.Question{
		{
			Name:   s,
			Qtype:  dns.TypeA,
			Qclass: dns.ClassINET,
		},
	}

	conn := new(dns.Conn)

	if conn.Conn, err = iface.DialTCP4(resolver); err != nil {
		return
	}
	c := new(dns.Client)

	return c.ExchangeWithConn(msg, conn)
}

func getHttpClient() *http.Client {
	netTransport := &http.Transport{
		DialContext: func(_ context.Context, network, add string) (net.Conn, error) {
			glog.V(2).Infof("Resolving IP to dial %v", add)
			parts := strings.Split(add, ":")
			if len(parts) != 2 {
				// Dial is only called with the host:port (no scheme, no path)
				return nil, fmt.Errorf("expected host:port but got %q", add)
			}
			host, port := parts[0], parts[1]
			// Look up the hostname
			r, _, err := resolve(host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve host: %v", err)
			}
			if len(r.Answer) == 0 {
				return nil, fmt.Errorf("failed to resolve A records for host %q", host)
			}
			var ip net.IP
			if a, ok := r.Answer[0].(*dns.A); ok {
				ip = a.A
			} else {
				return nil, fmt.Errorf("expected A record but got %T %q", r.Answer[0], r.Answer[0])
			}
			target := fmt.Sprintf("%s:%s", ip, port)
			glog.V(2).Infof("Dialing %s", target)
			return iface.DialTCP4(target)
		},
		TLSClientConfig: &tls.Config{
			// TODO: determine some way to make client certs available
			// This isn't horrific here, as all of the data that is fetched will be
			// cryptographically verified at a higher layer, but still... it's nasty.
			InsecureSkipVerify: true,
		},
	}
	c := http.Client{
		Transport: netTransport,
		Timeout:   httpTimeout,
	}
	return &c
}
