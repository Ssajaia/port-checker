# port-checker

port-check checks whether TCP connections to `host:port` endpoints can be established. It is a single static Go binary with no dependencies beyond the Go standard library, intended for local troubleshooting, shell scripts and CI/CD pipelines.

## What it does and does not do

port-check resolves the host, opens a TCP connection, measures how long that took, and closes the connection immediately. It reports one of five results (see [Results](#results)).

A successful result means a TCP handshake completed. It does **not** mean the application behind the port is healthy, that TLS is configured correctly, or that requests will succeed. port-check sends no data.

Not implemented: port range scanning, UDP, HTTP or other application-level checks, monitoring or background operation, persistent storage.

## Installation

Requires Go 1.24 or newer.

```bash
git clone https://github.com/Ssajaia/port-checker portpeek
cd portpeek
go build -o portpeek .
```

On Windows the output file should be named `port-check.exe`:

```powershell
go build -o port-check.exe .
```

Run the tests with `go test ./...`.

## Usage

```
port-check [options] <host:port>
port-check [options] check <config-file>
```

Options:

| Option                 | Description                                                                                         |
| ---------------------- | --------------------------------------------------------------------------------------------------- |
| `--timeout <duration>` | Connection timeout, e.g. `500ms`, `3s`, `1m`. Default `5s`. Overrides the timeout in a config file. |
| `--json`               | Print results as JSON on stdout.                                                                    |
| `--version`            | Print the version and exit.                                                                         |
| `-h`, `--help`         | Print usage and exit.                                                                               |

Options may appear before or after the target. Use `--` to end option parsing. Durations need a unit (`5` is rejected, `5s` is accepted) and must be greater than zero.

### Single endpoint

```console
$ port-check localhost:8080
open  localhost:8080  0.8ms

$ port-check 127.0.0.1:9
refused  127.0.0.1:9  0.2ms  connection refused

$ port-check --timeout 3s no-such-host.invalid:443
dns_error  no-such-host.invalid:443  1.1ms  dial tcp: lookup no-such-host.invalid on 8.8.8.8:53: no such host
```

IPv6 literals need brackets: `port-check "[::1]:8080"`. Ports must be numeric (1-65535); service names such as `http` are not supported.

### Multiple endpoints

```console
$ port-check check targets.yaml
open       Local API    localhost:8080            0.3ms
refused    Closed port  127.0.0.1:9               0.1ms  connection refused
dns_error  Bad host     no-such-host.invalid:443  1.1ms  dial tcp: lookup no-such-host.invalid on 8.8.8.8:53: no such host
3 checked: 1 open, 2 failed
```

Checks run concurrently, at most 16 at a time. Output always follows the order of the targets in the file, regardless of which check finishes first. A name column appears when at least one target has a name; unnamed targets show `-`. The error text for DNS failures comes from the Go resolver and varies by system.

## Configuration file

The `check` command reads a small subset of YAML. port-check does not use a YAML library, so only the constructs below are supported.

```yaml
targets:
  - name: Local API
    address: localhost:8080
  - name: HTTPS
    address: example.com:443
  - name: SSH
    address: example.com:22
timeout: 3s
```

Rules:

- `targets` is required and must list at least one item. Each item starts with `- ` and has an `address` (required) and an optional `name`.
- `timeout` is optional. Without it, and without `--timeout`, the default is 5 seconds. `--timeout` takes precedence over the file.
- Values may be wrapped in single or double quotes. Quotes are only stripped, with no escape processing.
- Full-line `#` comments and blank lines are ignored. Inline comments after a value are not supported and make the value invalid.
- Indent with spaces; tabs are rejected. List items may be indented or flush with `targets:`.
- Unknown keys, duplicate keys, duplicate names, and duplicate addresses are errors.
- Not supported: anchors and aliases, flow style (`[a, b]`, `{a: b}`), multi-line strings, multiple documents.

Errors include the file name and line number:

```console
$ port-check check bad.yaml
port-check: bad.yaml:2: target #1 is missing required field 'address'
```

## Results

| Status      | Meaning                                                                                                   |
| ----------- | --------------------------------------------------------------------------------------------------------- |
| `open`      | A TCP connection was established.                                                                         |
| `refused`   | The host answered and rejected the connection (nothing listening, or a firewall that actively rejects).   |
| `timeout`   | No answer arrived within the timeout. Often a firewall silently dropping packets, or an unreachable host. |
| `dns_error` | The hostname could not be resolved (including resolver failures and resolver timeouts).                   |
| `error`     | Any other failure, such as an unreachable network. The error text carries the details.                    |

The timeout covers the whole attempt, including DNS resolution. If a host resolves to several addresses, Go may try them in turn within that same timeout. Reported latency is the time until the connection was established or failed, including resolution.

On Windows, connecting to a closed port can take noticeably longer to report `refused` than on Linux or macOS, because the operating system retries before giving up.

## Exit codes

| Code | Meaning                                                               |
| ---- | --------------------------------------------------------------------- |
| `0`  | All requested checks succeeded.                                       |
| `1`  | One or more connectivity checks failed.                               |
| `2`  | Invalid arguments, invalid configuration, or another execution error. |

```bash
port-check --timeout 2s db.internal:5432 || echo "database port unreachable"
```

`--help` and `--version` exit with `0`.

## JSON output

`--json` prints one JSON document on stdout, with the same shape for single targets and `check`:

```console
$ port-check --json localhost:8080
{
  "results": [
    {
      "name": "",
      "address": "localhost:8080",
      "status": "open",
      "latency_ms": 0.45,
      "error": null
    }
  ],
  "summary": {
    "total": 1,
    "open": 1,
    "failed": 0
  }
}
```

- `name` is the configured name, or an empty string if none (always for single-target runs).
- `status` is one of the values in [Results](#results).
- `latency_ms` is a number in milliseconds, rounded to three decimals.
- `error` is `null` when the status is `open`, otherwise a string.
- `results` follows the order of the targets.

In JSON mode stdout contains only the JSON document. Argument, configuration and other errors go to stderr, stdout stays empty, and the exit code is `2`. In that case there is no JSON to parse, so check the exit code first.

## Supported operating systems

Linux, macOS and Windows. port-check uses only portable standard library networking and contains no platform-specific code besides recognizing the Windows "connection refused" error number.

## Limitations and security considerations

- TCP connectivity only; see [What it does and does not do](#what-it-does-and-does-not-do).
- Each check opens and immediately closes a real connection. The target service or an intrusion detection system may log it. Only check systems you are allowed to test.
- Targets come from the command line or the config file. Do not build a service that passes untrusted input to port-check: it would let that input probe internal hosts and ports from the machine running it.
- A middlebox such as a transparent proxy, load balancer or some firewalls can accept TCP connections on behalf of a backend that is down, so `open` does not prove the backend is reachable.
- `refused` and `timeout` are inferred from operating system errors and are a hint, not proof, of the cause.
- port-check does not read proxy environment variables; it always connects directly.

## License

MIT. See [LICENSE](LICENSE).
