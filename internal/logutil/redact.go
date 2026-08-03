package logutil

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"reg_go/internal/fileutil"
)

const redactionMarkerSuffix = ".redacted-v1"

var (
	emailRE       = regexp.MustCompile(`[A-Za-z0-9.!#$%&'*+/?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+`)
	otpValueRE    = regexp.MustCompile(`(?i)((?:验证码|otp|verification code)\s*[:=：]?\s*)[0-9]{4,8}`)
	secretValueRE = regexp.MustCompile(`(?i)(["']?(?:(?:\b(?:access[_-]?token|refresh[_-]?token|client[_-]?secret|device[_-]?code|api[_-]?key|user[_-]?code|workflow[_-]?state|reg[_-]?code|authorization[_-]?code|token|secret|password|code)\b)|(?:密码|令牌|密钥))["']?\s*[:=：]\s*["']?)([^"',\s}\]]+)`)
	proxyAuthRE   = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^\s/:@]+:)[^\s/@]+@`)
)

func Redact(msg string) string {
	msg = emailRE.ReplaceAllStringFunc(msg, func(email string) string {
		at := strings.LastIndexByte(email, '@')
		if at <= 0 {
			return "***"
		}
		return email[:1] + "***" + email[at:]
	})
	msg = otpValueRE.ReplaceAllString(msg, `${1}***`)
	msg = secretValueRE.ReplaceAllString(msg, `${1}***`)
	return proxyAuthRE.ReplaceAllString(msg, `${1}***@`)
}

func RedactFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	redacted := []byte(Redact(string(data)))
	if string(redacted) == string(data) {
		return nil
	}
	perm := fs.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	return fileutil.WriteFileAtomic(path, redacted, perm)
}

func RedactFileOnce(path string) error {
	fingerprint, err := fileFingerprint(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	marker, markerErr := os.ReadFile(path + redactionMarkerSuffix)
	if markerErr == nil && string(marker) == fingerprint {
		return nil
	}
	if err := RedactFile(path); err != nil {
		return err
	}
	return MarkFileRedacted(path)
}

func MarkFileRedacted(path string) error {
	fingerprint, err := fileFingerprint(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return fileutil.WriteFileAtomic(path+redactionMarkerSuffix, []byte(fingerprint), 0o600)
}

func fileFingerprint(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()), nil
}
