package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type preferenceStore interface {
	Read() (string, error)
	Save(context.Context, string, bool) (string, error)
}

type languagePreference struct {
	Schema   int    `json:"schema"`
	Language string `json:"language"`
}

func decodePreference(data []byte) (string, error) {
	var value languagePreference
	if len(data) > 4096 {
		return "", cliError("preference_invalid")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	// 拒绝重复键，避免损坏文件对语言或schema给出相互矛盾的定义。
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return "", cliError("preference_invalid")
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, e := decoder.Token()
		name, ok := key.(string)
		if e != nil || !ok || seen[name] {
			return "", cliError("preference_invalid")
		}
		seen[name] = true
		switch name {
		case "schema":
			e = decoder.Decode(&value.Schema)
		case "language":
			e = decoder.Decode(&value.Language)
		default:
			return "", cliError("preference_invalid")
		}
		if e != nil {
			return "", cliError("preference_invalid")
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || value.Schema != 1 || (value.Language != "en" && value.Language != "zh-CN") {
		return "", cliError("preference_invalid")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return "", cliError("preference_invalid")
	}

	return value.Language, nil
}

func (r *runner) preferenceWarning(code string) {
	fmt.Fprintf(r.err, "%s [%s]\n", r.errorMessage(cliError(code)), code)
}

func (r *runner) saveLanguage(language string, overwrite bool) string {
	if !r.canSave || r.preferences == nil {
		r.preferenceWarning("preference_readonly")
		return language
	}
	saved, err := r.preferences.Save(r.ctx, language, overwrite)
	if err != nil {
		r.preferenceWarning("preference_save_failed")
		return language
	}
	return saved
}
