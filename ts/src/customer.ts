/**
 * The customer page: your tenant's assets, and the public catalogue.
 *
 * It points at `custody`, the binary with **no policy** on it. A headquarters
 * credential arriving here is served its own tenant's rows like anybody else,
 * because there is nothing on that binary that answers anything wider -- which
 * is a thing to see rather than to be told, so this page will let you sign in
 * as `@hq/admin` and show you nothing.
 *
 * @module
 */

import { app, connect } from './client.js'
import { mount, run, table } from './ui.js'

/** Where the customer-facing server is; see `custody.yaml`. */
const ADDR = 'http://localhost:8080'

let who: string | null = localStorage.getItem('custody.who')

const client = app(connect(ADDR, () => who))

/**
 * And a second client that never signs anything.
 *
 * The catalogue is public, so calling it with whatever credential happens to be
 * in the box would hide the fact -- it would work signed in and nobody would
 * know whether it works signed out. Two clients is one line and says which
 * calls are which.
 */
const anonymous = app(connect(ADDR, () => null))

const $who = mount('who') as HTMLInputElement
const $assets = mount('assets')
const $catalogue = mount('catalogue')

$who.value = who ?? ''
$who.addEventListener('change', () => {
	who = $who.value.trim() || null
	if (who) {
		localStorage.setItem('custody.who', who)
	} else {
		localStorage.removeItem('custody.who')
	}

	assets()
})

/** tenant is the alias in `@tenant/holder`, which is what an Add has to name. */
function tenant(): string {
	return (who ?? '').replace(/^@/, '').split('/')[0] ?? ''
}

/**
 * assets is the walled read: no filter at all, and what comes back is what this
 * caller may see.
 *
 * It is the same request the admin page makes. Nothing here narrows it to a
 * tenant and nothing here could -- the wall is a predicate in the query, put
 * there by the server out of the credential, which is the whole point of it
 * being there rather than here.
 */
function assets(): void {
	run($assets, async () => {
		const res = await client.asset.list({})

		return table(
			['asset', 'name', 'where', 'listed'],
			res.items.map((v) => [v.alias, v.name, v.location, v.listed ? 'yes' : '']),
		)
	})
}

/**
 * add writes one, and the tenant it names is this caller's own.
 *
 * A tenant somebody else's is refused -- not by this page, which could say
 * anything, but by the layer that will not add a row to a tenant the caller
 * cannot see. Try it: sign in as one tenant and it is `NotFound` for the other,
 * which is the wall declining to say whether that tenant exists.
 */
mount('add').addEventListener('click', () => {
	const alias = (mount('alias') as HTMLInputElement).value.trim()
	const name = (mount('name') as HTMLInputElement).value.trim()
	const location = (mount('location') as HTMLInputElement).value.trim()
	const listed = (mount('listed') as HTMLInputElement).checked

	run($assets, async () => {
		await client.asset.add({
			tenant: { key: { case: 'alias', value: tenant() } },
			// Empty is not "no alias": the server generates a seven-letter one
			// when nothing is given, and folds and checks whatever is.
			alias,
			name,
			location,
			listed,
		})

		assets()
		catalogue()

		return document.createTextNode('')
	})
})

/**
 * catalogue is called with **no credential**, which is the whole of what makes
 * it the public axis.
 *
 * What comes back is four fields -- the identifier, the asset number, the name
 * and the description -- and that is a different message rather than an `Asset`
 * with things blanked out. Nobody sees the location or who is answerable for
 * it, and a field added to `Asset` tomorrow is published only by somebody
 * adding a line to the projection.
 */
function catalogue(): void {
	run($catalogue, async () => {
		const res = await anonymous.catalogue.search({})

		return table(
			['asset', 'name', 'about'],
			res.items.map((v) => [v.alias, v.name, v.desc]),
		)
	})
}

assets()
catalogue()
