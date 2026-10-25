# Home welcome facts

`HomeWelcome.svelte` chooses one welcome line when it mounts. Four lines are
playful taglines; four are paraphrased findings from peer-reviewed axolotl research.

## Citation convention

Use IEEE-style numbered citations, ordered by first appearance. Each scientific
fact has a matching reference number and DOI link beside it in the source. The
reference list includes authors, article title, journal, volume, issue when
available, page range or article number, year and DOI. When changing a fact,
update its source comment and this document together. Keep the wording within
the species, tissue and developmental stage studied.

## Facts

### Genome size

> An axolotl’s genome is about ten times the size of a human’s [1].

The paper reports a 32-gigabase-pair genome and explicitly compares it with the
human genome. The comparison concerns DNA length, not the number of genes.

### Immune cells and limb regeneration

> Axolotls need immune cells called macrophages to regrow a limb [2].

The experiments used axolotls. Depleting macrophages early after limb amputation
blocked regeneration despite wound closure; the requirement is timing-dependent.

### Forebrain regeneration

> Axolotls can replace lost neurons and rebuild connections in parts of their forebrain [3].

The study found restoration of neuron populations and axonal connections after
injury in the telencephalon. This does not establish regeneration of every brain
region or complete recovery of every brain function.

### A developmental limit on lens regeneration

> Axolotls lose the ability to regrow an eye lens about two weeks after hatching [4].

Lens regeneration was observed during a limited early developmental window,
roughly the first two weeks after hatching under the study conditions. This is an
approximate developmental window, not a universal cutoff at exactly fourteen days.

## References

[1] S. Nowoshilow _et al._, “The axolotl genome and the evolution of key tissue
formation regulators,” _Nature_, vol. 554, no. 7690, pp. 50–55, 2018, doi:
[10.1038/nature25458](https://doi.org/10.1038/nature25458).

[2] J. W. Godwin, A. R. Pinto, and N. A. Rosenthal, “Macrophages are required for
adult salamander limb regeneration,” _Proc. Natl. Acad. Sci. U.S.A._, vol. 110,
no. 23, pp. 9415–9420, 2013, doi:
[10.1073/pnas.1300290110](https://doi.org/10.1073/pnas.1300290110).

[3] K. Lust _et al._, “Single-cell analyses of axolotl telencephalon organization,
neurogenesis, and regeneration,” _Science_, vol. 377, no. 6610, 2022,
Art. no. eabp9262, doi:
[10.1126/science.abp9262](https://doi.org/10.1126/science.abp9262).

[4] R. Suetsugu-Maki _et al._, “Lens regeneration in axolotl: new evidence of
developmental plasticity,” _BMC Biol._, vol. 10, no. 1, 2012, Art. no. 103, doi:
[10.1186/1741-7007-10-103](https://doi.org/10.1186/1741-7007-10-103).

## Discovery layout

Home's left pane contains the welcome, file-drop control, Services, Storage and
Ingestion issues, in that order. Service cards fill the available list width.
Services is the primary open section of the shared resizable section stack and
scrolls independently. The main pane's shared header shows the selected service
and namespace. Collapse, expand and filter controls sit at the right of its
second row. The attribute table fills the space below it. The file-drop control
remains visible above the service list.
Attribute queries, typed values, sorting, filtering and exact Copy retain their
existing scope and behaviour. The header's filter icon opens and focuses the
attribute-key filter. Escape collapses the field and restores icon focus without
clearing the filter; an active filter remains marked on the icon.

Records is the number of distinct records carrying that attribute and type,
combining spans, logs and datapoints. Expanded rows count records carrying each
exact value. Those values can overlap on a record, so their counts are not
necessarily additive.

The table has four columns: Name, Records, %, and Copy. Received types appear
beside attribute names, with provenance on hover. The Attributes header offers
the same Collapse all / Expand all controls as traces. These apply across
attribute pages, including values still loading. Individual disclosures remain
independent until the next bulk action. Refreshing or changing the discovery
scope resets the disclosures.

Name uses the shared table's responsive pinned-prefix layout; Copy stays at
the far right. Narrow panes fall back to native horizontal scrolling when the
pinned prefix would leave too little room for another column. Resized widths
are saved.

Clicking an attribute or value row shows its matching Telemetry. Native name
and value buttons remain keyboard-accessible; a focused row also supports
Enter/Space selection and Up/Down/Home/End navigation. Copy, disclosures and
text selection do not activate the row.

Attributes offers 25, 50 or 100 entries per page, with each entry's expanded values
kept together. Filtering and sorting apply to all matching entries, not only the
current page. Changing pages preserves the selected attribute and expansion state.

The right-hand Details pane contains matching Telemetry. Its header's filter
menu chooses Traces, Metrics or Logs and shows each known matching-record count.
The active signal and count remain visible in the closed trigger. Choices switch
immediately; arrow, Home and End keys navigate available signals. Selecting a
choice or pressing Escape closes the menu and restores trigger focus.
Each signal retains independent pages and scroll positions. Attributes and
Telemetry each remember their own 25/50/100 page-size preference, defaulting to 25.
Selecting a service, attribute key or exact value updates the matching records;
record cards still navigate to their native signal workspace.
