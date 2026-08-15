# GTA Event Source Candidates v2 — Less-Visible Sources

Discovery round 2 (2026-08-15). Targets venues/orgs that do NOT rank in venue-guide
searches: catalog-driven enumeration (Artsdata SPARQL, Toronto Arts Council 2025
grantees, Ontario Arts Council 2025 results, Culture Days), ticketing-platform
mining (Eventbrite, Showpass), and retries of round-1 unvalidated candidates.

**DEDUP NOTE (orchestrator, 2026-08-15):** of the 78 quick-win rows below, **24 are
already configured** (918 Bathurst, Textile Museum, Toronto Summer Music, Toronto
Jazz Society, Aga Khan, Tarragon, Soulpepper, Crow's, Canadian Stage, Buddies,
Factory, Obsidian, Passe Muraille, Luminato, Small World, Charles Street Video,
Hot Docs, imagineNATIVE, InterAccess, Koffler, Mammalian, Opera Atelier, Wavelength,
Lula Lounge — all have configs in configs/sources/). music-toronto.com redirects to
the already-configured musictorontoconcerts.com. **53 quick wins are genuinely new** —
those are the ones to ticket. Existing-config overlap is likely because Artsdata/TAC
enumerate the same orgs round-1 research found; catalog discovery is best for the
long tail below the visible tier.

Method per candidate: `curl -sL --compressed --max-time 15` with browser UA, grep
for platform signals (JSON-LD / wp-content / tribe-events / Squarespace / Drupal /
Wix / Showpass / Eventbrite / `__NEXT_DATA__` / iCal). Est. tier: 0 = JSON-LD/ICS
feed, 1 = static HTML, 2 = headless/JS-rendered, 3 = REST API path.

All URLs fetched 2026-08-15. Provenance: [Artsdata] = SPARQL at
https://query.artsdata.ca/query (endpoint per docs/interop/artsdata.md — the
public URL is `query.artsdata.ca/query`, NOT `/sparql`); [TAC] = Toronto Arts
Council 2025 Grant Allocation Open Data (xlsx/csv zip, 953 rows,
torontoartscouncil.org/grant-recipients/); [OAC] = Ontario Arts Council 2025
grant results (arts.on.ca, program pages); [CD] = Culture Days; [EB] = Eventbrite
organizer pages; [SP] = Showpass; [retry] = round-1 re-test.

---

## Tier 0 / Tier 1 quick wins (catalog/platform finds, ranked by confidence)

| Name | URL (listing) | Platform | Est. tier | Est. events | Notes |
|------|---------------|----------|-----------|-------------|-------|
| 918 Bathurst | `https://918bathurst.com/events` | WordPress + Tribe Events | 0 | ~10/mo | Multi-arts venue; Tribe plugin + ICS signals (`?ical=1`). [Artsdata place] |
| Textile Museum of Canada | `https://textilemuseum.ca` | WordPress + Tribe + ICS | 0 | ~6–10 | Tribe events + iCal links on page. [TAC] |
| Toronto Mendelssohn Choir | `https://www.tmchoir.org` | WordPress + Tribe + ICS | 0 | ~8 | JSON-LD + tribe + ics; own performance calendar. [Artsdata] |
| Toronto Summer Music | `https://torontosummermusic.com` | WordPress + Tribe | 0–1 | ~25 | July festival; tribe + ics signals. [Artsdata] |
| Toronto Downtown Jazz Society | `https://torontojazz.com` | WordPress + Tribe + Showpass | 0–1 | ~40 | Jazz Fest + year-round; showpass ticketing links. [Artsdata] |
| Dance Ontario | `https://danceontario.ca` | WordPress + Tribe | 0–1 | ~15 | /calendar; umbrella org, captures member co. events. [Artsdata] |
| Xenia Concerts Inc. | `https://xeniaconcerts.com` | WordPress + Tribe + ICS | 0–1 | ~10 | Accessible concerts; tribe + ics. [OAC music] |
| Aga Khan Museum | `https://agakhanmuseum.org` | WordPress | 1 | ~15 | /whats-on; major cultural venue, not arts-only but events-rich. [TAC] |
| Tarragon Theatre | `https://tarragontheatre.com` | WordPress | 1 | ~15 | /calendar + /events + /tickets; major company. [TAC] |
| Soulpepper Theatre Company | `https://soulpepper.ca` | WordPress + Squarespace | 1 | ~20 | /calendar, /events, /season, /whats-on, event-listing classes. [Artsdata] |
| Crow's Theatre | `https://www.crowstheatre.com` | Static (custom) | 1 | ~10 | /calendar, /schedule, /shows hints. [Artsdata] |
| Canadian Stage | `https://www.canadianstage.com` | Static (custom) | 1 | ~15 | /calendar /events /season /shows. [Artsdata] |
| Buddies in Bad Times Theatre | `https://www.buddiesinbadtimes.com` | WordPress + Tribe + Showpass | 1 | ~10 | JSON-LD + tribe + showpass ticketing. [Artsdata] |
| Factory Theatre | `https://www.factorytheatre.ca` | WordPress (Cloudflare) | 1 | ~8 | BLOCKED to plain curl (CF) — flag for engine CF handling; JSON-LD present. [Artsdata] |
| Obsidian Theatre | `https://www.obsidiantheatre.com` | WordPress | 1 | ~5 | JSON-LD; /events + /season. [Artsdata] |
| Theatre Passe Muraille | `https://www.passemuraille.ca` | WordPress | 1 | ~8 | JSON-LD; /tickets. [Artsdata] |
| Roseneath Theatre | `https://www.roseneath.ca` | Wix | 1–2 | ~6 | Wix + react signals; youth theatre. [Artsdata] |
| Cahoots Theatre | `https://www.cahoots.ca` | Static | 1 | ~4 | /events + /tickets. [Artsdata] |
| Necessary Angel | `https://www.necessaryangel.com` | Squarespace | 1 | ~4 | JSON-LD. [Artsdata] |
| Outside the March | `https://outsidethemarch.ca` | WordPress | 1 | ~4 | JSON-LD. [Artsdata] |
| Citadel + Compagnie | `https://www.citadelcie.com/` | WordPress | 1 | ~6 | JSON-LD; /event/. [Artsdata] |
| BCurrent Performing Arts | `https://www.bcurrent.ca/` | Squarespace | 1 | ~5 | /events + /shows + /whats-on. [Artsdata] |
| Fall for Dance North | `https://www.ffdnorth.com` | Static | 1 | ~8 | Festival; static but structured. [Artsdata] |
| Toronto Jazz Orchestra | `https://thetjo.com` | Static + JSON-LD | 1 | ~12 | /events + /shows. [Artsdata] |
| Sinfonia Toronto | `https://www.sinfoniatoronto.com` | Static | 1 | ~6 | JSON-LD-ish; concert series. [Artsdata] |
| Toronto Choral Society | `https://www.torontochoralsociety.org` | WordPress + Tribe | 1 | ~6 | Tribe signals. [Artsdata] |
| Orchestra Toronto | `https://orchestratoronto.ca/` | WordPress | 1 | ~6 | JSON-LD; /tickets. [Artsdata] |
| Ontario Philharmonic | `https://www.ontariophil.ca` | Static | 1 | ~6 | /season. [Artsdata] |
| Toronto Youth Wind Orchestra | `https://tywo.ca/` | WordPress | 1 | ~8 | WordPress. [Artsdata] |
| Toronto Alliance for the Performing Arts | `https://tapa.ca` | WordPress + Tribe | 1 | ~10 | Umbrella org — captures member events + industry. [Artsdata] |
| Luminato Festival | `https://luminatofestival.com` | Squarespace | 1 | ~30 | June festival. [Artsdata] |
| Small World Music | `https://smallworldmusic.com` | Static + JSON-LD | 1 | ~10 | /events; world music presenter. [Artsdata] |
| Daniels Spectrum | `https://danielsspectrum.ca/` | Static + JSON-LD | 1 | ~8 | /whats-on; Regent Park arts hub. [Artsdata place] |
| Al Green Theatre | `https://www.mnjcc.org/agt` | Static | 1 | ~6 | /schedule; Miles Nadal JCC venue. [Artsdata place] |
| Jackman Performance Centre | `https://jackmanperformance.ca/` | WordPress | 1 | ~6 | /events. [Artsdata place] |
| Yonge-Dundas Square | `http://www.ydsquare.ca/` | WordPress | 1 | ~12 | /calendar + /whats-on; public square events. [Artsdata place] |
| Article 11 | `http://article11.ca` | WordPress | 1 | ~4 | Literary events. [Artsdata] |
| UofT Faculty of Music | `https://www.music.utoronto.ca` | Drupal | 1 | ~15 | /events; university concert series. [Artsdata] |
| Arraymusic | `https://arraymusic.com` | WordPress | 1 | ~10 | New-music series; /events + /season. [TAC] |
| Ashkenaz Foundation | `https://ashkenaz.ca` | WordPress | 1 | ~15 | Klezmer fest + year-round. [TAC] |
| Ballet Creole | `https://balletcreole.org` | WordPress | 1 | ~8 | Dance co. [TAC] |
| Breakthroughs Film Festival | `https://breakthroughsfilmfestival.com` | WordPress | 1 | ~8 | June festival. [TAC] |
| Charles Street Video | `https://charlesstreetvideo.com` | WordPress | 1 | ~6 | Media arts centre; /events. [TAC] |
| Corpus | `https://corpus.ca` | WordPress | 1 | ~8 | Dance; /calendar + /whats-on. [TAC] |
| Hand Eye Society | `https://handeyesociety.com` | WordPress + Eventbrite | 1 | ~10 | Games/arcade events; /event/ + eventbrite links. [TAC] |
| Hot Docs | `https://hotdocs.ca` | Static + JSON-LD | 1 | ~15 | Major doc festival + year-round screenings. [TAC] |
| imagineNATIVE | `https://imaginenative.org` | WordPress | 1 | ~12 | Indigenous media arts festival. [TAC] |
| InterAccess | `https://interaccess.org` | Static | 1 | ~8 | Media arts centre; /events + /season. [TAC] |
| Koffler Centre of the Arts | `https://kofflerarts.org` | Static | 1 | ~10 | /calendar. [TAC] |
| Mammalian Diving Reflex | `https://mammalian.ca` | WordPress + Vue | 1 | ~10 | Social-practice theatre; /calendar. [TAC] |
| Mayworks Festival | `https://mayworks.ca` | WordPress | 1 | ~12 | Labour arts festival. [TAC] |
| Music Toronto | `https://music-toronto.com` | Squarespace | 1 | ~12 | Chamber concert series. [TAC] |
| Nagata Shachu | `https://nagatashachu.com` | WordPress | 1 | ~8 | Taiko; /events. [TAC] |
| New Music Concerts | `https://newmusicconcerts.com` | WordPress | 1 | ~8 | /season + /tickets. [TAC] |
| Nia Centre for the Arts | `https://niacentre.org` | WordPress + Vue | 1 | ~10 | Black arts centre; /events. [TAC] |
| Opera Atelier | `https://operaatelier.com` | WordPress | 1 | ~10 | /shows + /whats-on. [TAC] |
| Planet in Focus | `https://planetinfocus.org` | WordPress + Vue | 1 | ~8 | Enviro film festival. [TAC] |
| Prologue to the Performing Arts | `https://prologue.org` | WordPress | 1 | ~10 | Touring children's shows. [TAC] |
| Regent Park Film Festival | `https://regentparkfilmfestival.com` | WordPress | 1 | ~8 | [TAC] |
| Shakespeare in the Ruff | `https://shakespeareintheruff.com` | WordPress | 1 | ~10 | Summer outdoor theatre. [TAC] |
| Soundstreams | `https://soundstreams.ca` | WordPress | 1 | ~10 | New-music presenter; /events + /season. [TAC] |
| Steps Public Art | `https://stepspublicart.org` | WordPress | 1 | ~8 | /events. [TAC] |
| Tangled Art + Disability | `https://tangledarts.org` | WordPress | 1 | ~8 | /events + /whats-on. [TAC] |
| Theatre Direct Canada | `https://theatredirect.ca` | WordPress | 1 | ~8 | /season; youth theatre. [TAC] |
| The Musical Stage Company | `https://musicalstagecompany.com` | WordPress | 1 | ~10 | /shows. [TAC] |
| The Word On The Street | `https://thewordonthestreet.ca` | WordPress | 1 | ~8 | Book fest. [TAC] |
| Toronto Blues Society | `https://torontobluessociety.com` | WordPress + Tribe | 1 | ~10 | /events + /season. [OAC music] |
| Toronto Tabla Ensemble | `https://torontotabla.com/events/` | WordPress | 1 | ~8 | [OAC music] |
| Wavelength | `https://wavelengthmusic.ca` | WordPress | 1 | ~12 | Indie music series; /event/ + /events. [OAC music] |
| Esprit Orchestra | `https://espritorchestra.com` | Squarespace | 1 | ~6 | [OAC music] |
| Against the Grain Theatre | `https://atgtheatre.com` | WordPress | 1 | ~6 | Experimental opera. [OAC music] |
| Hannaford Street Silver Band | `https://hssb.ca` | WordPress | 1 | ~8 | [OAC music] |
| Lula Lounge | `https://www.lula.ca/calendar` | Wix + Eventbrite | 1 | ~40 | World-music venue; /calendar + eventbrite links. [EB] |
| The Bentway | `https://www.thebentway.ca` | WordPress | 1 | ~15 | Public space under Gardiner; /whats-on + /event/. [EB] |
| Toronto Railway Museum | `https://torontorailwaymuseum.com` | WordPress + Tribe | 1 | ~10 | /events + /season. [EB] |
| Casa Loma | `https://casaloma.ca/events/` | WordPress + Vue | 1 | ~15 | [retry] |
| Toronto Zoo | `https://www.torontozoo.com/events` | Static | 1 | ~15 | [retry] |
| Native Earth Performing Arts | `https://nativeearth.ca/shows/all-shows/` | Squarespace | 1 | ~8 | [retry] |

## Tier 2 / headless candidates

| Name | URL (listing) | Platform | Est. events | Notes |
|------|---------------|----------|-------------|-------|
| Art Metropole | `https://artmetropole.com` | React | ~8 | /events; JS-rendered. [TAC] |
| Art Spin Toronto | `https://artspin.ca` | Wix + React | ~10 | [TAC] |
| Continuum Contemporary Music | `https://continuummusic.org` | Wix + React | ~8 | [TAC] |
| Dusk Dances | `https://duskdances.ca` | React | ~8 | /season. [TAC] |
| Mercer Union | `https://mercerunion.org` | Vue | ~8 | [TAC] |
| Pleiades Theatre | `https://pleiadestheatre.org` | Wix + React | ~6 | [TAC] |
| Shadowland Theatre | `https://shadowlandtheatre.ca` | Wix + Eventbrite | ~10 | [TAC] |
| RAW Taiko | `https://rawtaiko.ca` | Wix + React | ~8 | [OAC music] |
| Red Sky Performance | `https://www.redskyperformance.com/` | React | ~8 | /whats-on. [Artsdata] |
| ProArteDanza | `https://www.proartedanza.com` | Wix + React | ~6 | [retry] |
| CCDT | `https://ccdt.org` | Wix + Eventbrite | ~6 | Contemporary dance; eventbrite links. [retry] |
| The Mod Club (Axis Club successor) | `https://www.themodclub.com` | Next.js + JSON-LD | ~15 | theaxisclub.com now redirects here; /events + /event/ + /shows. [retry] |
| Lee's Palace | `https://www.leespalace.com` | Webflow (static render) | ~15 | /events + /event/ + /tickets; Webflow emits static HTML — may actually be Tier 1. [retry] |

## Tier 3 / API candidates (ticketing platforms)

| Name | URL | Platform | Est. events | Notes |
|------|-----|----------|-------------|-------|
| Eventbrite Toronto organizer pages (platform) | `https://www.eventbrite.ca/o/<slug>` | Eventbrite (Next.js `__NEXT_DATA__`) | varies | /o/ pages embed structured `upcomingEvents` in `__NEXT_DATA__` (verified on Lula Lounge: 12 events with start_date). T2 per scraper docs — one config could cover many orgs. [EB] |
| — Burdock Music Hall | `https://www.eventbrite.ca/o/burdock-music-hall-103809367271` | Eventbrite | ~15 | Also has own site (burdockbrewery.yaml exists — check overlap). [EB] |
| — Music Toronto | `https://www.eventbrite.ca/o/music-toronto-59613130173` | Eventbrite | ~12 | Duplicates music-toronto.com; EB as backup. [EB] |
| — Toronto Bach Festival | `https://www.eventbrite.ca/o/toronto-bach-festival-18386248073` | Eventbrite | ~8 | [EB] |
| — Toronto Concert Band | `https://www.eventbrite.ca/o/toronto-concert-band-12016971059` | Eventbrite | ~10 | [EB] |
| — The 519 | `https://www.eventbrite.ca/o/the-519-11100867914` | Eventbrite | ~15 | Community centre events. [EB] |
| — TPL Programs | `https://www.eventbrite.ca/o/tpl-programs-72000428503` | Eventbrite | ~30 | Toronto Public Library programs. [EB] |
| — SoCap Comedy Theatre | `https://www.eventbrite.ca/o/socap-comedy-theatre-6898984189` | Eventbrite | ~10 | [EB] |
| — Jokers Theatre & Comedy Club | `https://www.eventbrite.ca/o/jokers-theatre-comedy-club-65516133963` | Eventbrite | ~12 | [EB] |
| — Dance Hub Toronto | `https://www.eventbrite.ca/o/dance-hub-toronto-72600811973` | Eventbrite | ~10 | [EB] |
| — Goh Ballet Toronto | `https://www.eventbrite.ca/o/goh-ballet-toronto-46515927053` | Eventbrite | ~8 | [EB] |
| — Tablao Flamenco Toronto | `https://www.eventbrite.ca/o/tablao-flamenco-toronto-9902789664` | Eventbrite | ~6 | [EB] |
| Showpass Toronto (platform) | `https://www.showpass.com/discover/toronto/` | Showpass (JS SPA) | varies | Venue pages at `showpass.com/<slug>` (e.g. vanderpark → Burdock). T3 REST per scraper docs. [SP] |
| — Absolute Comedy | `https://www.showpass.com/absolute-comedy-toronto-...` | Showpass | ~15 | Tickets on showpass. [SP] |
| Showclix Toronto (platform) | `https://www.showclix.com/event/<slug>` | Showclix | varies | Used by Cineplex events, indie promoters in TO. T3 REST. [SP] |
| mhrth ticketing platform | `https://mhrth.com` | Custom (mhrth) | varies | Massey Hall / Roy Thomson Hall / Allied Music Centre all serve `*.mhrth.com` (403 to curl — bot-gated, likely API behind). One platform config. [Artsdata place] |

## Blocked / no events (one-line reason)

| Name | URL | Reason |
|------|-----|--------|
| Canadian Music Centre | `https://cmccanada.org` | Cloudflare 403. [TAC] |
| Dancemakers | `https://dancemakers.org` | Cloudflare 403. [TAC] |
| Inside Out | `https://insideout.ca` | Cloudflare 403 (2FA-challenge page served). [TAC] |
| Indigenous Fashion Arts | `https://indigenousfashionarts.com` | Cloudflare 403. [TAC] |
| Kaeja d'Dance | `https://www.kaeja.org` | Cloudflare 403. [TAC] |
| Studio 180 Theatre | `https://studio180theatre.com` | Cloudflare 403. [TAC] |
| Tafelmusik | `https://tafelmusik.org` | Cloudflare 403 (high value — revisit with CF handling). [TAC] |
| Tirgan Centre | `https://tirgan.ca` | Cloudflare 403. [TAC] |
| Toronto Operetta Theatre | `https://www.torontooperetta.com` | Cloudflare 403. [TAC] |
| Opera 5 | `https://www.opera5.ca` | Cloudflare 403. [TAC] |
| DesignTO | `https://designto.org` | Cloudflare 403 (note: also on Eventbrite). [TAC/EB] |
| Toronto Outdoor Art Fair | `https://toaf.ca` | Cloudflare 403 (also on Eventbrite). [TAC/EB] |
| Harbourfront Centre | `https://harbourfrontcentre.com/` | Cloudflare 403. [Artsdata] |
| Raag-Mala Toronto | `https://raagmala.ca` | Cloudflare 403. [Artsdata] |
| CanDance Network | `https://candance.ca` | Cloudflare 403. [Artsdata] |
| Indigenous Curatorial Collective | `https://icca.art/` | HTTP 500. [Artsdata] |
| Allied Music Centre | `https://alliedmusiccentre.mhrth.com/` | 403 bot-gate (see mhrth Tier 3 row). [Artsdata] |
| Alliance Française de Toronto | `https://www.alliance-francaise.ca/fr/` | Static, no events listing. [Artsdata] |
| Ontario Presents | `https://ontariopresents.ca/` | Drupal but org-level only, no events. [Artsdata] |
| Toronto Chinese Orchestra | `https://www.torontochineseorchestra.com/` | 392-byte page, no events. [Artsdata] |
| Canadian League of Composers | `https://www.composition.org/` | /shows hint but org/membership content, no dated listings. [Artsdata] |
| Adelheid | `https://adelheid.ca` | Static, no events. [Artsdata] |
| Anandam | `https://www.anandam.ca` | No events listing (Squarespace but static). [Artsdata] |
| Toronto Met School of Performance | `https://www.torontomu.ca/performance/` | Academic page, /season but no dated events. [Artsdata] |
| Evergreen (Brick Works) | `https://www.evergreen.ca` | No events section on homepage fetch (may be sub-page). [TAC] |
| Doris McCarthy Gallery | `https://www.utsc.utoronto.ca/dmg/` | Exhibitions only, no dated events. [TAC] |
| Craft Ontario | `https://craftontario.com` | No events listing. [TAC] |
| Little Pear Garden Dance Company | `https://lpgdc.com` | Squarespace, no events found. [TAC] |
| Eldritch Theatre | `https://eldritchtheatre.ca` | WP but /tickets only, no listing. [TAC] |
| Mixed Company Theatre | `https://mixedcompanytheatre.com` | WP, no events listing. [TAC] |
| Art Starts | `https://artstarts.ca` | Static, minimal. [TAC] |
| Kapisanan Philippine Centre | `https://kapisanan.com` | Parked/lander shell (114 bytes). [TAC] |
| Bad Hats Theatre | `https://badhatstheatre.com` | Squarespace, no dated events. [TAC] |
| Toronto History Museums (toronto.ca) | `https://www.toronto.ca/.../museums/` | Lives on toronto.ca shared Drupal calendar — needs shared config decision. [retry] |
| Black Creek Pioneer Village | `https://blackcreek.ca` | Cloudflare 403. [retry] |
| Culture Days (catalog) | `https://culturedays.ca/en/events` | Laravel SPA, JS-only shell; per-event `/ics` routes exist but no server-rendered list. Note as future T2. [CD] |
| OAC grant-results (catalog) | `https://www.arts.on.ca/grants/.../grant-results` | Server-rendered program pages OK (used here); month index pages are JS. Mostly non-GTA rural presenters. [OAC] |

## Track C retry verdicts (round-1 transport-error + low-priority lists)

| Name | Verdict | Platform | Listing? | Notes |
|------|---------|----------|----------|-------|
| Gallery 44 | **VIABLE** | Webflow | no dated events found | `gallery44.org` bare domain times out; `www.gallery44.org` works. Eventbrite links — check EB org. |
| FADO Performance Art Centre | **VIABLE** | WordPress + Tribe | `performanceart.ca/?post_type=tribe_events` 200 | Tribe events archive works. |
| Revival | **VIABLE** | WordPress + JSON-LD | `https://www.revivaleventvenue.ca/events` | Domain changed! revivalto.com dead → revivaleventvenue.ca (JSON-LD Place). |
| Baby G | **DEAD** | — | none | babygto.com DNS fail; www.babyg.ca is a parked lander (PW system). |
| Rivoli | **DEAD** | — | none | rivoli.ca DNS OK but connection refused (000). |
| Lee's Palace | **VIABLE** | Webflow | `https://www.leespalace.com/events` | Static HTML; /event/ + /tickets. |
| Axis Club | **VIABLE (rebranded)** | Next.js + JSON-LD | `https://www.themodclub.com` | axisclub.ca DNS dead; theaxisclub.com → themodclub.com. |
| Velvet Underground | **DEAD** | — | none | DNS resolves but no connection (000). |
| Cameron House | **DEAD** | — | none | cameronhouse.ca serves JS lander shell only (parking). |
| Dakota Tavern | **DEAD** | — | none | Domain parked for sale (GoDaddy). |
| Budweiser Stage | **BLOCKED** | Joomla (broken) | — | budweiserstage.com returns HTTP 500 PHP error; real listings on Live Nation (out of scope). |
| A Space Gallery | **VIABLE** | WordPress | `https://aspacegallery.org/events/` | 200; small gallery. |
| Prefix ICA | **VIABLE** | WordPress | `https://prefix.ca/events/` | 200. |
| Trinity Square Video | **VIABLE** | WordPress + Eventbrite | homepage 200 | /event/ + eventbrite links. |
| Native Earth | **VIABLE** | Squarespace | `https://nativeearth.ca/shows/all-shows/` | 200. |
| Volcano | **VIABLE** | Squarespace | homepage 200 | /events 404 but calendar links on site. |
| Bad Dog Theatre | **VIABLE** | Squarespace | homepage 200 | /shows 404 — listing is on homepage/season pages. |
| Aluna Theatre | **VIABLE** | WordPress | homepage 200 | Also on Eventbrite. |
| ProArteDanza | **VIABLE** | Wix | homepage 200 | T2. |
| CCDT | **VIABLE** | Wix + Eventbrite | homepage 200 | T2. |
| fu-GEN | **DEAD** | — | none | fugen.asia DNS fail (both www and bare). |
| Theatre Gargantua | **VIABLE** | WordPress + JSON-LD | homepage 200 | |
| Casa Loma | **VIABLE** | WordPress + Vue | `https://casaloma.ca/events/` | 200. |
| Black Creek Pioneer Village | **BLOCKED** | Cloudflare | — | 403. |
| Toronto Zoo | **VIABLE** | Static | `https://www.torontozoo.com/events` | 200; not arts-focused but events-rich. |
| Toronto History Museums | **PARTIAL** | toronto.ca Drupal | toronto.ca page 200 | Shared city calendar; needs shared-config decision, not per-site. |

---

## Notes / pipeline observations

- **Artsdata SPARQL**: use `https://query.artsdata.ca/query` (POST, `Content-Type:
  application/sparql-query`). The `/sparql` path is the human UI (Yasgui) and
  POSTs to `/sparql/` fail. Query: orgs with `schema:address/schema:addressLocality`
  in Toronto/Ontario or name regex, filtered to arts `@types`, `LIMIT 500` —
  returned 314 bindings / ~139 unique URLs. Places query returned 33 unique URLs
  (venues incl. 918 Bathurst, Daniels Spectrum, Massey Hall, St. Lawrence Centre).
- **TAC open data is the best single catalog**: 953 grantees (478 unique orgs) in
  one csv/xlsx zip — `2025-Grant-Allocation-TAC-Open-Data.zip` (needs Referer
  header to download; 403 without it). Could drive an automated org→website
  discovery pipeline.
- **OAC**: program result pages are server-rendered (recipient names parseable);
  month index pages are JS shells. Most music-org operating grantees are
  non-GTA; GTA subset captured above.
- **Eventbrite /o/ pages embed structured event data in `__NEXT_DATA__`** —
  verified: `pageProps.upcomingEvents[]` with `name`, `url`, `start_date`,
  `eventbrite_event_id`. A single Eventbrite organizer config pattern could
  unlock dozens of orgs.
- **mhrth.com** is the shared ticketing platform for Massey Hall / Roy Thomson
  Hall / Allied Music Centre (all `*.mhrth.com`); 403 to curl but likely has a
  REST API (T3 investigation candidate).
- **Culture Days** is a Laravel SPA with per-event ICS routes
  (`/events/{id}/ics`) but no server-rendered enumeration — T2/headless later.
- Cloudflare 403s are common on mid-size orgs (TAC list especially) — flagging
  candidates for the engine's CF handling would recover ~13 of them.

Generated by agent (t_a6e4b965), 2026-08-15.
