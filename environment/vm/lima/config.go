package lima

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

const configFile = "/etc/colima/colima.json"

// getConf reads the in-guest settings file.
//
// Fork change: the read error is returned rather than only trace-logged.
// Swallowing it meant a failed guest read surfaced downstream as
// `error retrieving current runtime: empty value` — a symptom naming neither
// the file nor the reason — which is how a cross-user read failure went
// misdiagnosed as an unfixable colima quirk for two months.
func (l limaVM) getConf() (map[string]string, error) {
	obj := map[string]string{}

	b, err := l.Read(configFile)
	if err != nil {
		return obj, fmt.Errorf("error reading %s in the VM: %w", configFile, err)
	}

	// we do not care if it fails
	_ = json.Unmarshal([]byte(b), &obj)

	return obj, nil
}

func (l limaVM) Get(key string) string {
	val, err := l.GetErr(key)
	if err != nil {
		l.Logger(context.Background()).Trace(err)
		return ""
	}

	return val
}

// GetErr is Get with the underlying read error preserved, for callers that
// must tell "unset" apart from "could not be read".
func (l limaVM) GetErr(key string) (string, error) {
	conf, err := l.getConf()
	if err != nil {
		return "", err
	}

	return conf[key], nil
}

func (l limaVM) Set(key, value string) error {
	obj, err := l.getConf()
	if err != nil {
		// Not fatal for a write — Write below recreates the file — but the
		// reason is worth recording.
		l.Logger(context.Background()).Trace(err)
	}
	obj[key] = value

	b, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("error marshalling settings to json: %w", err)
	}

	if err := l.Run("sudo", "mkdir", "-p", filepath.Dir(configFile)); err != nil {
		return fmt.Errorf("error saving settings: %w", err)
	}

	if err := l.Write(configFile, b); err != nil {
		return fmt.Errorf("error saving settings: %w", err)
	}

	return nil
}
