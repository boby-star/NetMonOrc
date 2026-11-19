# HTTP YAML: driver selection and target policy

HTTP checks accept `headers`, `connectIP` and `followRedirects`. GET/HEAD,
expectedStatus, bounded body, regex and extraction keep their existing semantics.
`verifyTLS: false` remains a validation error, as required by DSL §5.4.

```yaml
- id: application
  type: http
  runner: auto
  config:
    url: https://service.example/health
    connectIP: 192.0.2.10
    headers: {X-Probe: netmonorc}
    followRedirects: true
    verifyTLS: true
```

The pin must belong to the Agent's explicit allowed CIDRs. URL hostname remains
Host/SNI and certificate identity. Without a pin, native HTTP resolves the name
on the Agent, sorts addresses and dials only allowed literal IPs. The dialer
never gives the hostname to a second resolver. No environment proxy is used.

Redirects default to disabled. Native HTTP admits at most ten requests in one
bounded execution, applying URL/IP policy to every connection. Cross-origin
redirects lose custom headers; the pin applies only to the original hostname.
A denied hop produces execution_failed/unknown/target_policy_denied, preserving
coverage uncertainty rather than treating the target as down.

Header maps are limited to 32 entries / 8192 bytes, with string keys/values,
canonical case and duplicate detection. Credential and connection-control names,
CRLF/NUL and control characters are rejected. Values are not printed in errors.
Nonsecret headers also work in generated Blackbox modules.

The deployment deliberately selects native HTTP statically for `auto` with
connectIP, extraction, redirects or a DNS hostname. This extends the original
§5.5 auto-selection table so every socket can enforce target policy. Explicit
Blackbox with connectIP or redirects is rejected; explicit Blackbox still
requires a literal target IP. There is no fallback between drivers at runtime.

New semantics require Agent capability `http:policy-v2`, checked for regular
and diagnostic dispatch. Old Agents cannot silently ignore the new fields.
Default literal-IP checks retain their old serialized representation.

[Acceptance evidence](../experiments/http-policy-2026-11-19/README.md) includes
real Agent probes, private HTTPS API and PostgreSQL. Dedicated TLS socket tests
verify preserved Host/SNI and rejection of a certificate for another hostname.
This is not a wire-byte bound, full DNS sandbox or browser acceptance claim.
