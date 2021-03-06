package requestutil

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/url"

	"github.com/hashicorp/vault/helper/compressutil"
	"github.com/hashicorp/vault/helper/jsonutil"
)

type bufCloser struct {
	*bytes.Buffer
}

func (b bufCloser) Close() error {
	b.Reset()
	return nil
}

type ForwardedRequest struct {
	// The original method
	Method string `json:"method"`

	// The original URL object
	URL *url.URL `json:"url"`

	// The original headers
	Header http.Header `json:"header"`

	// The request body
	Body []byte `json:"body"`

	// The specified host
	Host string `json:"host"`

	// The remote address
	RemoteAddr string `json:"remote_addr"`

	// The client's TLS peer certificates
	PeerCertificates [][]byte `json:"peer_certificates"`
}

// GenerateForwardedRequest generates a new http.Request that contains the
// original requests's information in the new request's body.
func GenerateForwardedRequest(req *http.Request, addr string) (*http.Request, error) {
	fq := ForwardedRequest{
		Method:     req.Method,
		URL:        req.URL,
		Header:     req.Header,
		Host:       req.Host,
		RemoteAddr: req.RemoteAddr,
	}

	if req.TLS.PeerCertificates != nil && len(req.TLS.PeerCertificates) > 0 {
		fq.PeerCertificates = make([][]byte, len(req.TLS.PeerCertificates))
		for i, cert := range req.TLS.PeerCertificates {
			fq.PeerCertificates[i] = cert.Raw
		}
	}

	buf := bytes.NewBuffer(nil)
	_, err := buf.ReadFrom(req.Body)
	if err != nil {
		return nil, err
	}
	fq.Body = buf.Bytes()

	newBody, err := jsonutil.EncodeJSONAndCompress(&fq, &compressutil.CompressionConfig{
		Type: compressutil.CompressionTypeLzw,
	})
	if err != nil {
		return nil, err
	}

	ret, err := http.NewRequest("POST", addr, bytes.NewBuffer(newBody))
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// ParseForwardedRequest generates a new http.Request that is comprised of the
// values in the given request's body, assuming it correctly parses into a
// ForwardedRequest.
func ParseForwardedRequest(req *http.Request) (*http.Request, error) {
	buf := bufCloser{
		Buffer: bytes.NewBuffer(nil),
	}
	_, err := buf.ReadFrom(req.Body)
	if err != nil {
		return nil, err
	}

	var fq ForwardedRequest
	err = jsonutil.DecodeJSON(buf.Bytes(), &fq)
	if err != nil {
		return nil, err
	}

	buf.Reset()
	_, err = buf.Write(fq.Body)
	if err != nil {
		return nil, err
	}

	ret := &http.Request{
		Method:     fq.Method,
		URL:        fq.URL,
		Header:     fq.Header,
		Body:       buf,
		Host:       fq.Host,
		RemoteAddr: fq.RemoteAddr,
	}

	if fq.PeerCertificates != nil && len(fq.PeerCertificates) > 0 {
		ret.TLS = &tls.ConnectionState{
			PeerCertificates: make([]*x509.Certificate, len(fq.PeerCertificates)),
		}
		for i, certBytes := range fq.PeerCertificates {
			cert, err := x509.ParseCertificate(certBytes)
			if err != nil {
				return nil, err
			}
			req.TLS.PeerCertificates[i] = cert
		}
	}

	return ret, nil
}
