# Station names from the model, without guessing identity

Decision: 2026-10-25.

Reports must let users locate legs by full station names. A 3D model stores
station labels separately from leg geometry and can contain several names
and an anonymous node at the same position. Report every name at exactly the
endpoint coordinates and explicitly identify missing names.

Selecting one name by proximity or by survey would assert identity unsupported
by the input. Exact source SVX/SRV rows require a wider input scope and remain
outside the 3D/dump version. Preserve the independent anonymous-node indicator
when named and anonymous nodes coexist.
