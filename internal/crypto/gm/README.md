# 国密算法 (GM Crypto) Package

This package provides Chinese national cryptographic algorithm implementations as drop-in replacements for standard algorithms:

| Algorithm | Replaces | Usage |
|-----------|----------|-------|
| SM2 | RSA / ECDSA | Sign, Verify, Key Exchange |
| SM3 | SHA-256 | Hash |
| SM4-GCM | AES-256-GCM | Symmetric Encrypt / Decrypt |

## Build Tag

All files in this package are guarded by the `gm` build tag. To compile with GM support:

```bash
go build -tags=gm ./...
```

## Dependency

This package depends on [github.com/emmansun/gmsm](https://github.com/emmansun/gmsm) for the underlying SM2/SM3/SM4 implementations.

## Testing

```bash
go test ./internal/crypto/gm/... -tags=gm -count=1
```
