package auditapiserver

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// encodeContinueToken encodes an offset as a continue token.
func encodeContinueToken(offset int64) string {
	return base64.RawURLEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%d", offset)),
	)
}

// decodeContinueToken decodes a continue token back to an offset.
func decodeContinueToken(token string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("failed to decode continue token: %w", err)
	}
	offset, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid offset in continue token: %w", err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("negative offset in continue token")
	}
	return offset, nil
}
