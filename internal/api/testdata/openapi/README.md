# The OpenAPI 3.1 schema

`schema-2022-10-07.json` is the OpenAPI Initiative's JSON Schema for an
OpenAPI 3.1 document, as published at
<https://spec.openapis.org/oas/3.1/schema/2022-10-07>, copied unchanged on
2026-11-25. It is licensed under the Apache License 2.0, as the OpenAPI
Specification is.

SHA-256: `da01ba28852cac0de53893797cb8d1942bc3b05084f526dcc216717dec314ed0`

`TestTheContractIsAnOpenAPIDocument` validates `api/openapi.yaml` against it
(spec 113). This variant checks the document's structure and leaves its
schemas to the test's compiler, which checks each against JSON Schema 2020-12
when it compiles them. It is a test's fixture and is not built into hayami.
