# Survex Duplicate Detector glossary

These terms describe geometry preserved in an adjusted Survex model.

| Term | Meaning |
| --- | --- |
| Spatial model | Adjusted survey geometry in one common East/North/Up coordinate system. Extended elevation is not a spatial model for this detector. |
| Leg record | One occurrence of a segment, including endpoints and metadata. Identical occurrences retain separate identities. It is not a source reading row or a station. |
| Geometric duplicate candidate | An unordered pair of different eligible leg records whose endpoints match within tolerance in one orientation. Geometry alone does not prove redundant observations or station identity. |
| Endpoint tolerance | Inclusive maximum Euclidean error allowed for each matched endpoint. It is not a midpoint or average-error tolerance. |
| Relative tolerance | A percentage of the shorter leg's length, applied to each endpoint. It is not a statement of survey accuracy. |
| Absolute tolerance | An endpoint error limit in metres, independent of leg length. |
| Splay | A radial measurement from a traverse station to a wall or surrounding feature. Selection follows the SPLAY flag. |
| Acknowledged duplicate | A leg carrying Survex's DUPLICATE flag, indicating intentional repeat surveying. Excluded by default; included explicitly for a full audit. |
| Direct match | First endpoint to first endpoint, second endpoint to second endpoint. |
| Reversed match | First endpoint to second endpoint, second endpoint to first endpoint. |
| Endpoint error | Euclidean distance between a matched pair of endpoints, not an estimate of measurement accuracy. |
| Leg distance | The smaller orientation-specific maximum endpoint error. It is not the minimum distance between arbitrary points on segments. |
| Point leg | A leg with identical endpoints and zero length; it remains a leg record, not a station. |
| Survey | The survey label attached to a leg by the model. It need not identify a whole cave and is not a station name. |
| Station name | A full point label stored in the model. It is not a survey, leg ID or dump line number. |
| Names at an endpoint | All different nonempty full station names at exactly the endpoint's centimetre coordinates. They do not prove which name a source reading used. |
| Unnamed endpoint | An endpoint without a nonempty station name at its position. The other endpoint, coordinates and record metadata may still identify it. |
| Survey date | The leg's measurement date or date range, when present; not the model creation time. |
| Full dump | A model's complete text representation, including header and STOP. Its line numbers are not source SVX line numbers. |
