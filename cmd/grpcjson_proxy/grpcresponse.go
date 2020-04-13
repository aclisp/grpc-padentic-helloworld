package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

func handleErrResp(resp *http.Response, header http.Header) (*http.Response, bool) {
	if code := header.Get(headerGRPCStatusCode); code != "0" && code != "" {
		buff := bytes.NewBuffer(nil)
		grpcMessage := header.Get(headerGRPCMessage)
		j, _ := json.Marshal(grpcMessage)
		msg := string(j)
		buff.WriteString(`{"error":` + msg + `,"code":` + code + `}`)

		resp.Body = ioutil.NopCloser(buff)
		resp.StatusCode = 500

		return resp, true
	}
	return resp, false
}

func handleGRPCResponse(resp *http.Response) (*http.Response, error) {

	if resp, ok := handleErrResp(resp, resp.Header); ok {
		return resp, nil
	}

	prefix := make([]byte, 5)
	_, _ = resp.Body.Read(prefix)

	if resp, ok := handleErrResp(resp, resp.Trailer); ok {
		return resp, nil
	}

	resp.Header.Del(headerContentLength)

	return resp, nil

}
