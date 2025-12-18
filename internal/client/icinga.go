package client

import (
	"gms/icinga2otel/internal/config"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"container/ring"
)

var (
	hostRing *ring.Ring
)

func init() {
	hosts := config.Config.IcingaHosts

	hostRing = ring.New(len(hosts))

	for _, host := range hosts {
		hostRing = hostRing.Next()
		hostRing.Value = host
	}

}


func GetIcingaHost() string {
	hostRing = hostRing.Next()
	return hostRing.Value.(string)
}

func getTlsConfig() (*tls.Config) {

	certPool := x509.NewCertPool()

	if config.Config.IcingaCert != "" {
		if certPEM, err := os.ReadFile(config.Config.IcingaCert); err != nil {
			slog.Warn("Icinga Certfile was specified but could not be read.","error", err)
		} else {
			if ok := certPool.AppendCertsFromPEM(certPEM); !ok {
				slog.Warn("Icinga Certfile was specified but could not be added.")
			} else {
				slog.Info("Icinga certificate added.")
			}
		}
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify:  config.Config.IcingaInsecure,
                RootCAs: certPool,
	}

	//Custom Verify func to verify cert, but skip hostname check
	if config.Config.IcingaInsecureHost && !config.Config.IcingaInsecure {
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			opts := x509.VerifyOptions{
				DNSName: "", // skip
				Roots:   tlsConfig.RootCAs,
			}
			leaf, _ := x509.ParseCertificate(rawCerts[0])
			if _, err := leaf.Verify(opts); err != nil {
				return fmt.Errorf("failed to verify certificate against trusted CA: %v", err)
			}

			return nil
		}
	}

	if config.Config.IcingaClientCert != "" {
		if cert, err := tls.LoadX509KeyPair(config.Config.IcingaClientCert, config.Config.IcingaClientKey); err != nil {
			slog.Error("Icinga Client Certificate Problem","error",err)
		} else {
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	return tlsConfig

}

func GetIcingaHttpClient() (client *http.Client) {

	tr := &http.Transport{ TLSClientConfig: getTlsConfig() }

	client = &http.Client{
		Transport: tr,
	}

	return

}


