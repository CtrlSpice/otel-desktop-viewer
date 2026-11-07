package searchindex

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Entry struct {
	NativeText     *string
	NativeIdentity string
	Path           []any
	Kind           string
	Container      bool
	Attribute      bool
	KeyText        string
}
type Object struct {
	// NewOwner is set only for rows selected by a successful native append
	// attempt. Shared contexts are immutable; descriptors and Evals are not.
	NewOwner                                                                 bool
	Ref, Signal, Owner, RootKey, RecordKey, ParentRef, ResourceRef, ScopeRef string
	Entries                                                                  []Entry
}

func digest(parts ...string) string {
	h := sha256.New()
	var size [8]byte
	for _, part := range parts {
		binary.LittleEndian.PutUint64(size[:], uint64(len(part)))
		h.Write(size[:])
		h.Write([]byte(part))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }

func (o *Object) AddStrings(raw any) error {
	if raw == nil {
		return nil
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("native catalogue fields: unexpected %T", raw)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if fields[key] != nil {
			value, ok := fields[key].(string)
			if !ok {
				return fmt.Errorf("native catalogue field %s: unexpected %T", key, fields[key])
			}
			kind := "string"
			if key == "severityNumber" {
				kind = "int64"
			}
			o.Entries = append(o.Entries, Entry{Path: []any{key}, Kind: kind, NativeText: &value})
		}
	}
	return nil
}

// Write is called in the native mutation transaction. New telemetry owners use
// one modifying-CTE statement. Mutable owners use per-operation temporary
// staging, not retained cached results. Neither path uses triggers.
func Write(ctx context.Context, exec Executor, objects []Object) error {
	if len(objects) == 0 {
		return nil
	}
	unique := make(map[string]Object, len(objects))
	for _, o := range objects {
		unique[o.Ref] = o
	}
	objects = objects[:0]
	for _, o := range unique {
		objects = append(objects, o)
	}
	var refs, signals, owners, roots, records, parents, resources, scopes, fingerprints []string
	var linkRefs, linkPaths, linkValues []string
	appendOnly := true
	paths := map[string][]string{}
	values := map[string][]string{}
	for _, o := range objects {
		immutableContext := o.Signal == "" && (o.Owner == "resource" || o.Owner == "scope")
		if !o.NewOwner && !immutableContext {
			appendOnly = false
		}
		var identities []string
		for _, e := range o.Entries {
			path := jsonText(e.Path)
			pkey := digest(o.Owner, path, e.Kind)
			key := e.KeyText
			if len(e.Path) > 1 {
				relative := e.Path[1:]
				if e.Path[0] == "body" {
					relative = e.Path
				}
				if len(relative) == 1 {
					if s, ok := relative[0].(string); ok {
						key = s
					} else {
						key = jsonText(relative)
					}
				} else {
					key = jsonText(relative)
				}
			} else if len(e.Path) > 0 {
				key = fmt.Sprint(e.Path[0])
			}
			scope := o.Owner
			if len(e.Path) > 0 && e.Path[0] == "metadata" {
				scope = "metadata"
			}
			paths[pkey] = []string{o.Owner, path, e.Kind, key, e.KeyText, scope, fmt.Sprint(e.Attribute)}
			value := ""
			if e.NativeText != nil {
				value = *e.NativeText
			}
			vkey := digest(e.Kind, value)
			if e.NativeIdentity != "" {
				vkey = e.NativeIdentity
			}
			stored := value
			if e.Container {
				stored = ""
			}
			values[vkey] = []string{e.Kind, stored}
			linkRefs = append(linkRefs, o.Ref)
			linkPaths = append(linkPaths, pkey)
			linkValues = append(linkValues, vkey)
			identities = append(identities, pkey+vkey)
		}
		sort.Strings(identities)
		refs = append(refs, o.Ref)
		signals = append(signals, o.Signal)
		owners = append(owners, o.Owner)
		roots = append(roots, o.RootKey)
		records = append(records, o.RecordKey)
		parents = append(parents, o.ParentRef)
		resources = append(resources, o.ResourceRef)
		scopes = append(scopes, o.ScopeRef)
		fingerprints = append(fingerprints, digest(o.Signal, o.Owner, o.RootKey, o.RecordKey, o.ParentRef, o.ResourceRef, o.ScopeRef, strings.Join(identities, "")))
	}
	if appendOnly {
		return writeNewOwners(ctx, exec, [][]string{refs, signals, owners, roots, records, parents, resources, scopes, fingerprints}, paths, values, linkRefs, linkPaths, linkValues)
	}
	run := func(q string, args ...any) error { _, err := exec.ExecContext(ctx, q, args...); return err }
	if err := run(`create or replace temp table search_incoming_owners as
	 select i.* from (select unnest(?::varchar[]) ref,unnest(?::varchar[]) signal,unnest(?::varchar[]) owner,
	 unnest(?::varchar[]) root_key,unnest(?::varchar[]) record_key,unnest(?::varchar[]) parent_ref,
	 unnest(?::varchar[]) resource_ref,unnest(?::varchar[]) scope_ref,unnest(?::varchar[]) fingerprint) i
	 left join search_owners old using(ref) where old.ref is null or old.fingerprint<>i.fingerprint`, refs, signals, owners, roots, records, parents, resources, scopes, fingerprints); err != nil {
		return err
	}
	deleted, err := exec.ExecContext(ctx, `delete from search_postings where owner_id in(select o.id from search_owners o join search_incoming_owners i using(ref))`)
	if err != nil {
		return err
	}
	oldCount, err := deleted.RowsAffected()
	if err != nil {
		return err
	}
	if err := run(`insert into search_owners(ref,signal,owner,root_key,record_key,parent_ref,resource_ref,scope_ref,fingerprint)
	 select ref,signal,owner,root_key,record_key,nullif(parent_ref,''),nullif(resource_ref,''),nullif(scope_ref,''),fingerprint from search_incoming_owners
	 on conflict(ref) do update set signal=excluded.signal,owner=excluded.owner,root_key=excluded.root_key,record_key=excluded.record_key,parent_ref=excluded.parent_ref,resource_ref=excluded.resource_ref,scope_ref=excluded.scope_ref,fingerprint=excluded.fingerprint`); err != nil {
		return err
	}
	var pi, po, pp, pk, pkey, pt, ps, pa []string
	for id, p := range paths {
		pi = append(pi, id)
		po = append(po, p[0])
		pp = append(pp, p[1])
		pk = append(pk, p[2])
		pkey = append(pkey, p[3])
		pt = append(pt, p[4])
		ps = append(ps, p[5])
		pa = append(pa, p[6])
	}
	if len(pi) > 0 {
		if err := run(`insert into search_paths(identity,owner,path,kind,key,key_text,attribute_scope,is_attribute)
	 select i.* from (select unnest(?::varchar[]) identity,unnest(?::varchar[]) owner,unnest(?::varchar[])::json path,
	 unnest(?::varchar[]) kind,unnest(?::varchar[]) key,unnest(?::varchar[]) key_text,unnest(?::varchar[]) attribute_scope,unnest(?::varchar[])::boolean is_attribute) i
	 where not exists(select 1 from search_paths old where old.identity=i.identity)`, pi, po, pp, pk, pkey, pt, ps, pa); err != nil {
			return err
		}
	}
	var vi, vk, vv []string
	for id, v := range values {
		vi = append(vi, id)
		vk = append(vk, v[0])
		vv = append(vv, v[1])
	}
	if len(vi) > 0 {
		if err := run(`insert into search_values(identity,kind,value)
	 select i.* from(select unnest(?::varchar[]) identity,unnest(?::varchar[]) kind,unnest(?::varchar[]) value) i
	 where not exists(select 1 from search_values old where old.identity=i.identity)`, vi, vk, vv); err != nil {
			return err
		}
	}
	if len(linkRefs) > 0 {
		if err := run(`insert into search_postings
	 select distinct o.id,p.id,v.id from(select unnest(?::varchar[]) ref,unnest(?::varchar[]) path,unnest(?::varchar[]) value) i
	 join search_incoming_owners changed using(ref) join search_owners o using(ref)
	 join search_paths p on p.identity=i.path join search_values v on v.identity=i.value`, linkRefs, linkPaths, linkValues); err != nil {
			return err
		}
	}
	if oldCount > 0 {
		return CollectGarbage(ctx, exec)
	}
	return nil
}
