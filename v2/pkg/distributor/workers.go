package distributor

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"net/http"

	"github.com/openshift/oc-mirror/v2/pkg/api/v1alpha3"
	clog "github.com/openshift/oc-mirror/v2/pkg/log"
)

const (
	CONTENTTYPE     string = "Content-Type"
	APPLICATIONJSON string = "application/json"
)

// RemoteWorkerInterface
type RemoteWorkerInterface interface {
	ReadWorkerCsv(string) ([]string, error)
	CheckServices([]string) error
	ProcessWorkload([]string, []v1alpha3.CopyImageSchema) error
	makeRequest(string, string, []byte) ([]byte, error)
}

// New - instantiate the implementation of the interface
func New(log clog.PluggableLoggerInterface) RemoteWorkerInterface {
	return &RemoteWorker{Log: log}
}

// RemoteWorker - schema
type RemoteWorker struct {
	Log clog.PluggableLoggerInterface
}

// ReadWorkerCsv - a comma separated value filew with a list of ips (address:port)
func (o *RemoteWorker) ReadWorkerCsv(file string) ([]string, error) {
	var addresses []string
	data, err := os.ReadFile(file)
	if err != nil {
		return addresses, err
	}
	ips := strings.Split(string(data), ",")
	for _, str := range ips {
		adr := strings.TrimSpace(str)
		o.Log.Trace("address %s ", adr)
		addresses = append(addresses, adr)
	}
	o.Log.Trace("total server count %d ", len(addresses))
	return addresses, nil
}

// CheckServices - ping function to ensure all services are responding
func (o *RemoteWorker) CheckServices(ips []string) error {
	// run series as we want to fail quick
	for _, ip := range ips {
		if len(ip) > 0 {
			url := "http://" + ip + "/api/v2/isalive"
			o.Log.Info("checking server at address %s ", url)
			_, err := o.makeRequest(http.MethodGet, url, nil)
			if err != nil {
				return fmt.Errorf("service at ip %s did not respond this will degrade performance", ip)
			}
		}
	}
	return nil
}

// ProcessWorkload - read json payload and call the copy/mirror function
func (o *RemoteWorker) ProcessWorkload(ips []string, images []v1alpha3.CopyImageSchema) error {
	workload := len(images) / len(ips)
	o.Log.Debug("images count %d", len(images))
	o.Log.Debug("workers %d", len(ips))
	o.Log.Debug("workloads count %d", workload)
	var payload []v1alpha3.CopyImageSchema
	for idx, ip := range ips {
		if len(ip) > 0 {
			url := "http://" + ip + "/api/v1/batch"
			o.Log.Info("sending batch payload to %d", url)
			if idx == len(ips)-1 {
				payload = images[(idx)*workload:]
			} else {
				payload = images[(idx * workload):((idx + 1) * workload)]
			}
			b, _ := json.MarshalIndent(payload, "", "	")
			_, err := o.makeRequest(http.MethodPost, url, b)
			if err != nil {
				return fmt.Errorf("service at ip %s did not respond please verify", ip)
			}
		}
	}
	return nil
}

// makeRequest - private utility function for GET and POST
func (o *RemoteWorker) makeRequest(method string, url string, data []byte) ([]byte, error) {
	var b []byte

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: tr}

	if method == http.MethodGet {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return b, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return b, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return []byte("ok"), nil
		}
		return []byte("ko"), fmt.Errorf(strconv.Itoa(resp.StatusCode))
	}
	if method == http.MethodPost {
		req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
		if err != nil {
			return b, err
		}
		req.Header.Add("Content-Type", APPLICATIONJSON)

		resp, err := httpClient.Do(req)
		if err != nil {
			return b, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return b, err
		}
		o.Log.Trace("response payload %s", string(body))

		if resp.StatusCode == http.StatusOK {
			return []byte("ok"), nil
		}
		return []byte("ko"), fmt.Errorf(strconv.Itoa(resp.StatusCode))
	}
	return []byte("ok"), nil
}
