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

One SBOM per image, made by your own tools; vessel never runs a scanner.

```sh
vessel build ./acme --key vessel.key --layout ./bundle -o scratch
vessel inspect ./bundle                                   # each image and the digest it is pinned to
syft docker.io/library/nginx@sha256:… -o spdx-json > nginx.spdx.json
vessel build ./bundle --key vessel.key --evidence nginx.spdx.json -o myapp
./myapp inspect --evidence ./audit                        # on site
```

Scan each image by the digest `inspect` prints, never by its tag: a tag can move between the build and the scan.
