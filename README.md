# ocr

Tesseract OCR as an HTTP service. No GPU, so it schedules on any amd64 node.

```
POST /ocr  ->  tesseract CLI  ->  ocr.dev.initialed85.cc
```

Live on `home-dev`, namespace `ocr`, image `initialed85/ocr` (built on the x86
daemon, `build.sh`).

## API

| Route | Input | Output |
| --- | --- | --- |
| `GET /` | — | tesseract version, languages, counters |
| `GET /healthz` | — | `{"ok":true}` |
| `POST /ocr` | `file` (multipart) **or** `url` (form), optional `lang`, `psm`, `format` | text, TSV or JSON |

`format` is `txt` (default), `tsv` or `json`. `json` is TSV parsed into words with
confidence and bbox, which is what you want when you're asserting on OCR output.

Both POST routes take `-F` (multipart/form-data); `-d` will not work.

```shell
# file upload
curl -s -F "file=@scan.png" https://ocr.dev.initialed85.cc/ocr

# remote image
curl -s -F "url=https://example.com/photo.jpg" https://ocr.dev.initialed85.cc/ocr

# structured output with per-word confidence
curl -s -F "file=@scan.png" -F "format=json" https://ocr.dev.initialed85.cc/ocr

# a uniform block of text (screenshot, sign) instead of full page segmentation
curl -s -F "file=@scan.png" -F "psm=6" https://ocr.dev.initialed85.cc/ocr
```

`psm` is Tesseract's page segmentation mode; the default is `3` (automated). `6`
is a single uniform block and is usually better for screenshots.

## Pairs with chromium

The two services validate each other: render a page with chromium, screenshot it,
OCR the screenshot, assert the text. See `~/Projects/Home/chromium/validate.sh`.

## Validate

```shell
./validate.sh                                   # against the live service
BASE_URL=http://127.0.0.1:8080 ./validate.sh    # against a local container
```

## Notes

- The `url` fetch happens inside the pod, so it can reach anything the cluster can
  reach. No allowlist — fine on the trusted network, worth restricting if this
  ever leaves it.
- Tesseract reads PNG/JPEG/TIFF/PNM/BMP. It does not read HTML, so posting an HTML
  `url` returns 500.
- `MAX_UPLOAD_BYTES` (default 10 MB) and `OCR_TIMEOUT_SECONDS` (default 30) are
  enforced.
