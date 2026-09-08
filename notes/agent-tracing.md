# Agent tracing blog notes

## The abomination prompt

```text
You are working inside the `otel-desktop-viewer` repository. This is a read-only literary commission for a private, non-public parody and literary experiment.

Before composing, inspect the README, documentation, manifests, representative source files, and tests. Determine what the application actually does, how telemetry enters and moves through it, how traces and spans are represented, and which details distinguish it from a generic OpenTelemetry viewer. Privately identify enough verifiable technical details to sustain the entire poem.

Treat the current source as authoritative if documentation and implementation differ. Do not modify any files. Do not invent features, components, APIs, bugs, architectural history, maintainer anecdotes, or behavior unsupported by the repository.

Then write a mock-epic narrative poem about `otel-desktop-viewer`.

FORM

The work must consist of a proem, four seasonal cantos, and an epilogue. Together, the cantos form a cycle. Give the poem an extravagantly ceremonious mock-epic title, preferably with an unnecessary subtitle following "or."

Begin each canto with a single pompous prose "Argument" that foretells its events with considerably more grandeur than circumstances warrant.

Each canto should contain roughly 24 to 40 lines of rhymed narrative verse and enough action to advance an actual plot. The proem and epilogue may be shorter. Do not abbreviate the later cantos into summaries.

PRIMARY INSPIRATION

Take Alexander Pushkin's `Ruslan and Lyudmila` as the primary stylistic inspiration for the main poem: buoyant rhymed narrative verse, fairy-tale velocity, heroic spectacle, romantic absurdity, conversational irony, and a narrator visibly delighted by their own artifices.

Favor nimble iambic tetrameter, but preserve wit, momentum, and musicality rather than producing mechanical doggerel.

SECONDARY INSPIRATIONS

These references have strictly assigned responsibilities. They are not equal ingredients in a literary smoothie.

Nicolas Boileau's `Le Lutrin` supplies mock-heroic disproportion. Minor questions of placement, interface, and procedure should be narrated as though ecclesiastical order, dynastic legitimacy, and the architecture of the cosmos depended upon them.

Byron's `Don Juan` supplies the cultivated and intrusive narrator who digresses, confides, boasts, changes the subject, and occasionally punctures their own sublimity.

Ariosto's `Orlando Furioso` supplies the architecture of quests, interruptions, improbable encounters, suspended revelations, and canto endings poised over narrative abysses.

Nabokov's `Pale Fire` supplies one structural conceit only: annotations that gradually cease behaving like servants of the poem and begin composing a rival account of events.

Use Terry Pratchett's Discworld footnotes as the direct stylistic model for the technical annotations, particularly the institutional satire, explanatory digressions, and humane absurdity found in `Going Postal`, `The Truth`, and `Small Gods`.

The desired effect is not generic British whimsy. Use Pratchett's characteristic comic architecture: begin with a precise factual objection, follow its logic into bureaucracy, folklore, class interest, institutional vanity, or historical accident, and conclude with a sober observation that detonates the entire digression retroactively.

Treat software infrastructure as Pratchett treats civic institutions: indispensable arrangements created by fallible people, subsequently mistaken for natural law.

Produce an original poem. Do not quote these authors or reconstruct recognizable passages from their work. The sole exception permitting a borrowed character and associated lore is described below.

THE SEASONAL CYCLE

Let the Pacific Northwest turn through an esoteric olfactory calendar rather than postcard scenery or familiar petrichor.

Winter smells of storm-thrown kelp, wet wool, and cold iron on a ferry rail.

Spring smells of black-cottonwood resin, damp bark, and salmonberry bruised underfoot.

Summer smells of sun-struck Douglas-fir pitch, blackberry dust, and grass cured almost to incense.

Autumn smells of chanterelle loam, collapsing maple leaves, distant woodsmoke, and the mineral chill preceding frost.

These scents must participate in the narrative rather than merely decorate it. Let their transformations register shifts in latency, memory, accumulation, decay, visibility, or causality. Include one extended epic simile in each canto, grounded in the ecology or material life of the Pacific Northwest.

Do not use petrichor.

THE QUEST

The poem must have an actual plot derived from the repository, not merely a procession of observability metaphors. Select a real unit of telemetry, interaction, or causality as the basis for an epic protagonist only after inspecting the code.

Follow its passage through the system, allowing its journey to assume the dignity and inconvenience of an epic quest. Give it obstacles, reversals, recognition, and consequence. Technical fidelity must survive the metaphorical transformation.

Include one invocation to an appropriately improbable Muse.

Include one heroic catalogue assembled from real project components, modules, concepts, or interfaces.

Include one katabasis through nested causality.

Include at least one minor technical operation narrated as though kingdoms will fall if it is delayed.

Let the repository determine the mechanics and stakes. Metaphorical exaggeration is welcome; false technical claims are not.

THE TRIOLET APPARATUS

Attach exactly one primary superscript footnote to each seasonal canto. Use the superscript numerals ¹, ², ³, and ⁴. Place each corresponding note immediately after its canto so the joke does not depend upon automatic Markdown footnote rendering.

Every primary footnote must be a strict triolet with rhyme scheme ABaAabAB.

Lines 1, 4, and 7 must repeat exactly.

Lines 2 and 8 must repeat exactly.

Each triolet must perform four tasks without appearing to perform any of them.

It must communicate at least one accurate, repository-grounded technical detail.

It must object to, qualify, or embarrass the heroic claim carrying its superscript marker.

It must pursue one perfectly logical premise considerably farther than good judgment recommends.

It must use repetition as comic escalation. The first appearance of a refrain should sound plausible, the second suspiciously bureaucratic, and the third like a verdict delivered under a regulation nobody remembers approving.

Let the humor arise from exact mechanisms rather than generic programmer jokes. A technical term may be interpreted with ruinous literalness. A naming convention may be treated as constitutional law. An implementation compromise may be explained as the outcome of several abstractions possessing incompatible definitions of the word "obvious." A small interface decision may reveal a complete and regrettably functioning system of rank.

The satire should punch toward pomposity, institutional inertia, status, false certainty, and systems behaving exactly as designed. It must never sneer at users, novices, or people encountering the project for the first time. The humor should remain humane even when the machinery is not.

Do not invent bugs, commit history, maintainer anecdotes, or fictional project behavior. The analogy may be absurd; the technical mechanism beneath it must be true.

Permit exactly one of the four triolets to contain a superscript sub-footnote. It must appear within the spring triolet. Mark it with ᵃ and place it immediately beneath its parent triolet. The sub-footnote must consist of one technically accurate sentence whose pedantic necessity is initially doubtful and ultimately undeniable. Do not nest any farther. Civilization has limits.

THE LIBRARIAN EXCEPTION

One deliberate exception to the prohibition on borrowed characters is permitted: the Librarian of Unseen University may appear as himself.

Relevant Discworld lore may accompany him, including L-space, his orangutan form, his professional dignity, his preference for "Ook," and the categorical inadvisability of calling him a monkey. Do not borrow any other Discworld characters.

The Librarian must perform a structural function rather than make a decorative cameo. He should first enter the poem through sub-footnote ᵃ, having discovered that repositories contain libraries, traces may contain trees, and footnotes offer dangerously inadequate access controls.

Treat L-space as the undocumented compatibility layer among libraries, repositories, and literary annotation. At least one footnote should insist that this is not a pun but an architectural dependency. L-space is a deliberately fictional intrusion; do not imply that it is an actual feature of the repository.

After entering through the spring sub-footnote, the Librarian should begin auditing the poem's technical claims. His interpretations of "branch," "root," "stack," "tree," and "span" may be zoologically or professionally literal, but the repository details beneath the jokes must remain accurate.

In the summer canto, allow him to move from the sub-footnote into the primary triolet apparatus.

By autumn, permit him to cross into the main narrative while the increasingly unreliable narrator is briefly relegated to annotation. This reversal should complete the footnotes' rise from commentary to rival authority.

The Librarian may speak only through "Ook" and its typographical variations. Any translation must be supplied by a footnote whose confidence substantially exceeds its qualifications.

He should ultimately prove to be the character best equipped to navigate nested causality, because he alone understands that all sufficiently complicated systems become libraries and all insufficiently documented libraries become adventures.

Do not let the Librarian displace `otel-desktop-viewer` as the true subject. He is its external auditor, the poem's accidental psychopomp, and the instrument by which the narrative discovers that its own hierarchy is observable.

THE SHADOW CYCLE

Taken together, the four footnote triolets must form a second narrative beneath the first.

The winter note behaves like a respectful technical gloss.

The spring note begins to suspect that the narrator has confused confidence with documentation. The Librarian enters through its sub-footnote.

The summer note discovers contradictions, material omissions, or suspiciously heroic simplifications. The Librarian has now entered the primary apparatus and commenced his audit.

The autumn note has instrumented the poem containing it and can report upon the narrator as an unreliable service. The Librarian crosses into the principal narrative while the narrator is demoted into their own annotation.

Allow images and verbal residues to migrate between the four triolets, but preserve the exact internal repetitions required by the form.

By the epilogue, the footnotes should not merely comment upon the mock epic. They should constitute a quieter and more accurate epic beneath it. The observability apparatus should become more trustworthy than the heroic voice it was originally employed to annotate.

The poem itself must become observable.

REGISTER

Keep the main verse lexically sumptuous, ceremonious, sensuous, technically alert, and faintly insufferable. Its grandiloquence should display judgment and precision rather than indiscriminate thesaurus abuse.

Keep the footnotes exact, dry, lateral, and devastating. Their jokes must arise from genuine technical details.

Avoid stock technological mysticism involving "tapestries," "symphonies," "beacons," "digital realms," "unseen threads," or "the dance of data."

Do not produce product copy, documentation with line breaks, a tour of repository features, or a procession of identical trace-and-span puns.

Return only the titled poem, its canto Arguments, and its triolet footnotes. Do not include a repository summary, citations, creative rationale, or explanation of your process.
```
