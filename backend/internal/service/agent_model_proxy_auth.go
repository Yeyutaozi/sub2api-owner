package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
)

const agentModelProxyInternalAuthHeader = "X-Sub2API-Agent-Internal-Auth"

var agentModelProxyInternalAuthKey = mustGenerateAgentModelProxyInternalAuthKey()

var agentModelProxySignedHeaders = [...]string{
	"User-Agent",
	"X-Sub2API-Agent-App-ID",
	"X-Sub2API-Agent-App-Version-ID",
	"X-Sub2API-Agent-Run-ID",
	"X-Sub2API-Agent-Node-ID",
	"X-Sub2API-Agent-Node-Role",
	"X-Sub2API-Agent-Model-Group-ID",
	"X-Sub2API-Model",
}

func mustGenerateAgentModelProxyInternalAuthKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("generate agent model proxy internal auth key: " + err.Error())
	}
	return key
}

func SignAgentModelProxyInternalRequest(req *http.Request) {
	if req == nil {
		return
	}
	req.Header.Set(agentModelProxyInternalAuthHeader, agentModelProxyInternalSignature(req))
}

func VerifyAgentModelProxyInternalRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	if req.Header.Get("User-Agent") != "Sub2API-Agent-ModelProxy/1.0" {
		return false
	}
	actual, err := base64.RawURLEncoding.DecodeString(req.Header.Get(agentModelProxyInternalAuthHeader))
	if err != nil || len(actual) != sha256.Size {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(agentModelProxyInternalSignature(req))
	return err == nil && hmac.Equal(actual, expected)
}

func agentModelProxyInternalSignature(req *http.Request) string {
	mac := hmac.New(sha256.New, agentModelProxyInternalAuthKey)
	writeAgentModelProxySignatureField(mac, req.Method)
	path := ""
	if req.URL != nil {
		path = req.URL.EscapedPath()
	}
	writeAgentModelProxySignatureField(mac, path)
	for _, name := range agentModelProxySignedHeaders {
		writeAgentModelProxySignatureField(mac, req.Header.Get(name))
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func writeAgentModelProxySignatureField(mac hashWriter, value string) {
	_, _ = mac.Write([]byte(value))
	_, _ = mac.Write([]byte{0})
}

type hashWriter interface {
	Write([]byte) (int, error)
}
