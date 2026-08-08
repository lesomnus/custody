/**
 * The headquarters page: every tenant's assets, and moving one between them.
 *
 * It points at `custody-admin`, which is a **different address** and that is
 * the only difference in this file that matters. The calls are the same calls
 * the customer page makes -- `AssetService.List` with no filter -- and what
 * comes back is every tenant's rows instead of one tenant's, because that
 * binary was handed a policy and this one was not.
 *
 * Splitting the page in two is not a security decision, and it is worth being
 * clear about that: anything this page can do, `curl` can do. The wall is on
 * the server. What the split buys is that each page points at one address and
 * shows the screens that address has, and that the customer bundle does not
 * carry a transfer form for an endpoint it cannot reach.
 *
 * @module
 */

import { app, connect } from './client.js'
import { el, mount, run, table } from './ui.js'

/** Where the headquarters server is; see `custody-admin.yaml`. */
const ADDR = 'http://localhost:8081'

let who: string | null = localStorage.getItem('custody.admin.who')

const client = app(connect(ADDR, () => who))

const $who = mount('who') as HTMLInputElement
const $assets = mount('assets')
const $out = mount('out')

$who.value = who ?? ''
$who.addEventListener('change', () => {
	who = $who.value.trim() || null
	if (who) {
		localStorage.setItem('custody.admin.who', who)
	} else {
		localStorage.removeItem('custody.admin.who')
	}

	assets()
})

/**
 * assets is every tenant's, and the request says nothing about tenants.
 *
 * `Hq.Where` answers "every tenant" and the credential is met with it, so an
 * operator whose credential names one customer sees that customer and nobody
 * else -- from this same page, with this same call. Sign in as a customer's
 * holder and the server refuses outright: `permission_denied`, from `Hq.May`,
 * before a row is read.
 */
function assets(): void {
	run($assets, async () => {
		const res = await client.asset.list({})

		// A read each, and it is not a page being careless. A `List` answers
		// with rows and **not with their edges** -- every `tenant` in that
		// answer is null -- so which tenant a row is in is a second read, with
		// a `select` that names it. The customer page never needs one, because
		// every row it can see is in the same tenant; this page is the one
		// where the answer differs per row, which is the whole of what it is
		// here to show.
		const rows = await Promise.all(
			res.items.map((v) =>
				client.asset.get({
					ref: { key: { case: 'id', value: v.id } },
					select: { all: true, tenant: { alias: true } },
				}),
			),
		)

		return table(
			['asset', 'name', 'where', 'tenant'],
			rows.map((v) => [
				v.alias,
				v.name,
				v.location,
				el('code', {}, v.tenant?.alias || '?'),
			]),
		)
	})
}

/**
 * transfer moves an asset to another tenant.
 *
 * It is an RPC written by hand rather than a `Patch`, and the reason is in
 * `proto/ext/app/asset_svc.ext.proto`: `Asset.tenant` is mutable, so a general
 * write could set it, which is exactly why the general writes are closed. The
 * rule -- a reason somebody can read in a year, a destination the caller can
 * see -- lives in the one door.
 */
mount('transfer').addEventListener('click', () => {
	const alias = (mount('ref') as HTMLInputElement).value.trim()
	const from = (mount('from') as HTMLInputElement).value.trim()
	const to = (mount('to') as HTMLInputElement).value.trim()
	const reason = (mount('reason') as HTMLInputElement).value.trim()

	run($out, async () => {
		const v = await client.asset.transfer({
			ref: {
				key: {
					case: 'slug',
					value: {
						alias,
						tenant: { key: { case: 'alias', value: from } },
					},
				},
			},
			to: { key: { case: 'alias', value: to } },
			reason,
		})

		assets()

		return el('p', { class: 'ok' }, `${v.alias} is now ${v.tenant?.alias ?? to}'s`)
	})
})

assets()
