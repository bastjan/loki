# `encoding` stage

The `encoding` stage is a transform stage that allows decoding arbritary encodings to UTF8.

## Schema

```yaml
encoding:
  # Encoding to use when decoding the data or log line.
  encoding: <string>

  # Name from extracted data to decode. If empty, uses the log message.
  # The decoded value will be assigned back to source key.
  [source: <string>]
```

### Supported Encodings

The Following encodings are supported through golang.org/x/text/encoding/charmap:

- IBM Code Page 037
- IBM Code Page 437
- IBM Code Page 850
- IBM Code Page 852
- IBM Code Page 855
- Windows Code Page 858
- IBM Code Page 860
- IBM Code Page 862
- IBM Code Page 863
- IBM Code Page 865
- IBM Code Page 866
- IBM Code Page 1047
- IBM Code Page 1140
- ISO 8859-1
- ISO 8859-2
- ISO 8859-3
- ISO 8859-4
- ISO 8859-5
- ISO 8859-6
- ISO-8859-6E
- ISO-8859-6I
- ISO 8859-7
- ISO 8859-8
- ISO-8859-8E
- ISO-8859-8I
- ISO 8859-9
- ISO 8859-10
- ISO 8859-13
- ISO 8859-14
- ISO 8859-15
- ISO 8859-16
- KOI8-R
- KOI8-U
- Macintosh
- Macintosh Cyrillic
- Windows 874
- Windows 1250
- Windows 1251
- Windows 1252
- Windows 1253
- Windows 1254
- Windows 1255
- Windows 1256
- Windows 1257
- Windows 1258
- X-User-Defined

## Example

### Without `source`

Given the pipeline:

```yaml
- encoding:
    encoding: "Windows 1252"
```

And the log line:

```
2019-01-01T01:00:00.000000001Z stderr P i'm a log message from an ancient \xf0 mac \x11!
```

The log line becomes

```
2019-01-01T01:00:00.000000001Z stderr P i'm a log message from an ancient \xf0 mac \x11!
```

### With `source`

Given the pipeline:

```yaml
- json:
    expressions:
     level:
     msg:
- encoding:
    source:   "msg"
    encoding: "Windows 1252"
```

And the log line:

```
{"time":"2019-01-01T01:00:00.000000001Z", "level": "info", "msg":"11.11.11.11 - "POST /loki/api/push/ HTTP/1.1" 200 932 "-" "Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6"}
```

The first stage would add the following key-value pairs into the `extracted`
map:

- `time`: `2019-01-01T01:00:00.000000001Z`
- `level`: `info`
- `msg`: `11.11.11.11 - "POST /loki/api/push/ HTTP/1.1" 200 932 "-" "Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6"`

While the encoding stage would then parse the value for `msg` in the extracted map
and encodings the `msg` value. `msg` in extracted will now become

- `msg`: `11.11.11.11 - "POST /loki/api/v1/push/ HTTP/1.1" 200 932 "-" "Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6"`
