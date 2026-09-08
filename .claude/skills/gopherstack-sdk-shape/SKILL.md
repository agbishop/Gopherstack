---
name: gopherstack-sdk-shape
description: Look up the authoritative AWS wire shape (protocol, request/response fields, HTTP binding, error set) for a gopherstack service operation by reading the pinned aws-sdk-go-v2 source in the module cache, instead of guessing or trusting a handler's existing output. Trigger when adding/fixing an operation, verifying a wire.go struct, writing a PARITY.md wire/errors entry, chasing a "wrong shape" or "500 InternalFailure" bug, or whenever you're about to write AWS request/response field names, error codes, or timestamp formats from memory.
---

# gopherstack-sdk-shape

Never infer a wire shape from a sibling operation or from field names — read each
operation's own serializer/deserializer. Same-looking ops can differ: directconnect's
`AllocatePrivateVirtualInterface` flattens `VirtualInterface` fields onto Output while
`AllocateTransitVirtualInterface` nests it as `VirtualInterface *types.VirtualInterface`.

## Fast path: use the script

```bash
.claude/skills/gopherstack-sdk-shape/scripts/sdkshape.sh <service>            # protocol + op list
.claude/skills/gopherstack-sdk-shape/scripts/sdkshape.sh <service> <Operation> # file + function locations
```

No-op-name mode resolves the pinned version from `go.mod`, prints the SDK dir,
detects the protocol from the serializer prefix, and lists `api_op_*.go` files
(count + first 20). With an operation, it prints the `api_op_<Op>.go` path plus
the serializer/deserializer/error-deserializer function names with line numbers.

That covers steps 1–3 below. Read the printed files yourself for the actual field
list — the script locates, it doesn't summarize.

## The manual recipe (what the script automates)

1. `ls $(go env GOMODCACHE)/github.com/aws/aws-sdk-go-v2/service/<svc>@<ver>/api_op_<Op>.go`
   — get `<ver>` from `go.mod` (`grep "aws-sdk-go-v2/service/<svc> " go.mod`); multiple
   versions coexist in the cache, always use the one pinned in this repo's go.mod.
   Input/Output structs live here with `// This member is required` markers.
2. `grep -n "^func " serializers.go | grep -i <Op>` — protocol prefix + which
   binding functions exist for this op.
3. `grep -n "func aws.*_deserializeOpError<Op>" deserializers.go` and read that
   op's own switch statement — the authoritative per-op exception set.
4. REST protocols only: `grep -n "SplitURI\|request.Method" serializers.go` to
   extract the real HTTP verb + path-parameter pattern for the op.

## Protocol-prefix decoder

The serializer function name prefix tells you the protocol — nothing else to guess:

| prefix | protocol | notes |
|---|---|---|
| `awsRestjson1_*` | REST-JSON | path/header/query bindings + JSON body; check `serializeOpHttpBindings<Op>Input` for what is NOT in the body |
| `awsEc2query_*` | EC2-query | form-encoded `Action=<Op>&...`, XML response, field encoding via `query.Value` |
| `awsAwsjson11_*` / `awsAwsjson10_*` | JSON-RPC | `X-Amz-Target` header dispatch, no URL routing — confirm with `grep -n "X-Amz-Target" serializers.go` (e.g. directconnect is `POST /` with `X-Amz-Target: OvertureService.<Op>`) |
| `awsRestxml_*` | REST-XML | |
| `awsAwsquery_*` | AWS Query/XML | not `awsQuery_*` — that prefix does not exist in generated code |

Two services have no standard-prefixed `serializers.go` at all and need hand-reading
instead of a grep: cloudwatch (rpc-v2-cbor via `options.Protocol = rpcv2.NewCBOR(...)`
in `api_client.go`) and appstream (generated `serializeCBOR_*`/`deserializeCBOR_*`
functions with no `awsXxx_` prefix). `sdkshape.sh` reports "unknown" for both — that's
correct, not a bug.

See `services/_PROTOCOLS.md` for the full per-service protocol table (all 161
services, resolved from the pinned SDKs) if you want to skip re-deriving a
service's protocol from scratch.

## Why `types/errors.go` misleads

That file enumerates every exception shape the *SDK package* can ever produce
across all its operations — it does not say which ops actually raise which
errors. Only the per-op `awsRestjson1_deserializeOpError<Op>` (or equivalent)
switch in `deserializers.go` is authoritative for a given operation. Wire the
gopherstack error path against that switch, not the shared type list.

## Gotcha checks (exact greps)

- **XML list wrapper `<member>` vs `<Item>`**: check the query serializer's
  array-encoding call for the field vs REST-XML's `xml:"...>member"` tag in
  `types/types.go` — confirm against the specific op, not a sibling.
- **Epoch-seconds vs ISO8601**: JSON-protocol services often wire timestamps as
  epoch-seconds floats. Grep the field's line in serializers/deserializers for
  `.Double(` vs `.String(smithytime.FormatDateTime(...))`. Emit with
  `pkgs/awstime.Epoch(t time.Time) float64` (`pkgs/awstime/awstime.go:24`).
- **httpPayload response-root nesting**: does the Output struct have an
  httpPayload-tagged single member that changes whether fields are flattened
  onto the response root or nested under one key? Check the specific op's
  Output struct in `api_op_<Op>.go`, not a sibling with a similar name.
- Governing rule, repeated in every A-grade PARITY.md: one op's shape never
  transfers to a same-looking sibling. Read each op's own serializer and
  deserializer every time.

## Downstream use

Once you have the shape: `gopherstack-service-op` covers turning it into
`wire.go`/`wire_convert.go` edits and wiring the handler; `gopherstack-parity-audit`
covers recording the verification in PARITY.md.
