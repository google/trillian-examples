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
	"time"

	"github.com/golang/glog"
	"github.com/miekg/dns"
	usbnet "github.com/usbarmory/imx-usbnet"
	"github.com/usbarmory/tamago/soc/nxp/imx6ul"
)

const (
	deviceIP  = "10.0.0.1"
	deviceMAC = "1a:55:89:a2:69:41"
	hostMAC   = "1a:55:89:a2:69:42"
	resolver  = "8.8.8.8:53"
)

var iface *usbnet.Interface

func initNetworking() error {
	var err error

	iface, err = usbnet.Init(deviceIP, deviceMAC, hostMAC, 1)

	if err != nil {
		return fmt.Errorf("could not initialize USB networking, %v", err)
	}

	iface.EnableICMP()

	imx6ul.USB1.Init()
	imx6ul.USB1.DeviceMode()
	imx6ul.USB1.Reset()
	return nil
}

func runNetworking() error {
	// never returns
	imx6ul.USB1.Start(iface.Device())

	return nil
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
