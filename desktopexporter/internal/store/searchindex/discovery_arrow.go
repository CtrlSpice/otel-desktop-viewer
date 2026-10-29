package searchindex

import "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"

// The catalogue stores paths as JSON. Project its segments to native columns so
// numeric indices and string keys remain distinct without browser JSON parsing.
const nativePath = `list_transform(from_json(path, '["JSON"]'), lambda segment: struct_pack(
    key := case when json_type(segment)='VARCHAR' then json_extract_string(segment,'$') end,
    index := case when json_type(segment)!='VARCHAR' then segment::UBIGINT end))`

const discoveryOrder = `match_count desc,key,attribute_scope,kind,path`

func DiscoverArrowSQL(options DiscoveryOptions) (string, []any, error) {
	q, args, err := discoverySQL(options, false, discoveryOwners("$1"))
	if err != nil {
		return "", nil, err
	}
	projection := `select key as name,attribute_scope as attributeScope,owner,` + nativePath + ` as path,kind as type,match_count as matchCount,`
	if len(options.Path) == 0 {
		return `with matches as (` + q + `) ` + projection + `[]::VARIANT[] as sampleValues from matches order by ` + discoveryOrder, args, nil
	}
	signal, _ := NormalizeSignal(options.Signal)
	document, from, identity, err := nativeSampleSource(options.Owner, options.Path, signal)
	if err != nil {
		return "", nil, err
	}
	params := make([]search.NamedParam, len(args))
	for i, arg := range args {
		params[i] = search.NamedParam{Value: arg}
	}
	value, err := search.ResolveValue(document, options.Path[1:], "", &params)
	if err != nil {
		return "", nil, err
	}
	for _, param := range params[len(args):] {
		args = append(args, param.Value)
	}
	// At most three representatives per matched typed path, fetched together
	// from native owners. Ranking and content identity are shared with RPC.
	query := discoveryOwners("$1") + `, matches as (` + q + `), representatives as (
 select l.path_id,min(o.record_key) as record,
 row_number() over(partition by l.path_id order by v.value,v.identity) as ordinal
 from search_postings l join matches m on m.id=l.path_id
 join discovery_owners o on o.id=l.owner_id join search_values v on v.id=l.value_id
 where v.value ilike $5 or ($5='%%' and v.kind in('map','array','empty'))
 group by l.path_id,v.identity,v.value qualify ordinal<=3
), native_samples as (
 select r.path_id,r.ordinal,` + value.ValueSQL + ` as value from ` + from + `
 join representatives r on ` + identity + `=r.record
 qualify row_number() over(partition by r.path_id,r.ordinal)=1
), samples as (
 select path_id,list(value order by ordinal) as sampleValues from native_samples group by path_id
) ` + projection + `coalesce(sampleValues,[]::VARIANT[]) as sampleValues
 from matches left join samples on samples.path_id=matches.id order by ` + discoveryOrder
	return query, args, nil
}
