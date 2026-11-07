# Signing

```sh
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out vessel.key
openssl pkey -in vessel.key -pubout -out vessel.pub       # copy once to every site
vessel build ./acme --key vessel.key -o myapp             # writes myapp and myapp.sig
```

On site, with no network:

```sh
base64 -d myapp.sig > myapp.der
openssl dgst -sha256 -verify vessel.pub -signature myapp.der myapp
```

## SBOM

```sh
vessel build ./acme --key vessel.key --layout ./bundle -o scratch
syft ./bundle -o spdx-json > sbom.spdx.json
vessel build ./bundle --key vessel.key --evidence sbom.spdx.json -o myapp
./myapp inspect --evidence ./audit                        # on site
```
