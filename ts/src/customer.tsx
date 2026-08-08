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

import { useState } from 'react'
import { createRoot } from 'react-dom/client'

import { Provider, useCall, useQuery } from '@lesomnus/payday/react'
import { key } from '@lesomnus/payday/store'

import { AssetService } from '../gen/app/asset_svc_pb.js'
import { CatalogueService } from '../gen/app/catalogue_pb.js'
import { Opened, Say, Signed, Table, useCredential, useOpened } from './ui.js'
import './style.css'

/** Where the customer-facing server is; see `custody.yaml`. */
const ADDR = 'http://localhost:8080'

/**
 * Assets is the walled read: no filter at all, and what comes back is what this
 * caller may see.
 *
 * It is the same request the admin page makes. Nothing here narrows it to a
 * tenant and nothing here could -- the wall is a predicate in the query, put
 * there by the server out of the credential, which is the whole point of it
 * being there rather than here.
 *
 * The `Watch` beside it is opened for as long as this is on screen, so an asset
 * somebody else adds or moves out of your tenant arrives without this page
 * asking again.
 */
function Assets(): React.ReactNode {
	const { data, error } = useQuery(AssetService.method.list, {})

	if (error !== undefined) return <Say err={error} />
	if (data === undefined) return <p className="empty">...</p>

	return (
		<Table head={['asset', 'name', 'where', 'listed']} empty={data.items.length === 0}>
			{data.items.map((v) => (
				<tr key={key(v.id)}>
					<td>{v.alias}</td>
					<td>{v.name}</td>
					<td>{v.location}</td>
					<td>{v.listed ? 'yes' : ''}</td>
				</tr>
			))}
		</Table>
	)
}

/**
 * Add writes one, and the tenant it names is this caller's own.
 *
 * A tenant somebody else's is refused -- not by this page, which could say
 * anything, but by the layer that will not add a row to a tenant the caller
 * cannot see. Try it: sign in as one tenant and name the other, and it is
 * `NotFound`, which is the wall declining to say whether that tenant exists.
 *
 * Nothing here refreshes the list. The row the write answered with goes into
 * the store, and the lists over assets are read again -- because a create can
 * change what belongs in one and only the server knows which.
 */
function Add(props: { tenant: string }): React.ReactNode {
	const [v, setV] = useState({ alias: '', name: '', location: '', listed: false })
	const add = useCall(AssetService.method.add)

	return (
		<>
			<form
				className="form"
				onSubmit={(e) => {
					e.preventDefault()
					add.call({ tenant: { key: { case: 'alias', value: props.tenant } }, ...v })
						.then(() => setV({ alias: '', name: '', location: '', listed: false }))
						.catch(() => {})
				}}
			>
				{/* Empty is not "no alias": the server generates a seven-letter
				    one when nothing is given, and folds and checks whatever is. */}
				<input
					value={v.alias}
					placeholder="asset number, or leave empty"
					size={24}
					onChange={(e) => setV({ ...v, alias: e.target.value })}
				/>
				<input
					value={v.name}
					placeholder="name"
					size={24}
					onChange={(e) => setV({ ...v, name: e.target.value })}
				/>
				<input
					value={v.location}
					placeholder="where it is"
					size={24}
					onChange={(e) => setV({ ...v, location: e.target.value })}
				/>
				<label>
					<input
						type="checkbox"
						checked={v.listed}
						onChange={(e) => setV({ ...v, listed: e.target.checked })}
					/>{' '}
					in the catalogue
				</label>
				<button type="submit" disabled={add.state === 'pending'}>
					add
				</button>
			</form>
			{add.state === 'error' && <Say err={add.error} />}
		</>
	)
}

/**
 * Catalogue is called with **no credential**, which is the whole of what makes
 * it the public axis.
 *
 * It reads through a store of its own, and that is the point rather than a
 * detail: a store is opened for a credential, and this one is opened for none.
 * Calling it with whatever is in the box above would hide the fact -- it would
 * work signed in and nobody would know whether it works signed out.
 *
 * What comes back is four fields -- the identifier, the asset number, the name
 * and the description -- and that is a different message rather than an `Asset`
 * with things blanked out. Nobody sees the location or who is answerable for
 * it, and a field added to `Asset` tomorrow is published only by somebody
 * adding a line to the projection.
 */
function Catalogue(): React.ReactNode {
	const { data, error } = useQuery(CatalogueService.method.search, {})

	if (error !== undefined) return <Say err={error} />
	if (data === undefined) return <p className="empty">...</p>

	return (
		<Table head={['asset', 'name', 'about']} empty={data.items.length === 0}>
			{data.items.map((v) => (
				<tr key={key(v.id)}>
					<td>{v.alias}</td>
					<td>{v.name}</td>
					<td>{v.desc}</td>
				</tr>
			))}
		</Table>
	)
}

function Anonymous(props: { children: React.ReactNode }): React.ReactNode {
	const app = useOpened('anon', ADDR, null)
	if (app === undefined) return <p className="empty">...</p>

	return <Provider app={app}>{props.children}</Provider>
}

/** tenant is the alias in `@tenant/holder`, which is what an Add has to name. */
function tenantOf(who: string | null): string {
	return (who ?? '').replace(/^@/, '').split('/')[0] ?? ''
}

function Customer(): React.ReactNode {
	const [who, setWho] = useCredential('customer')

	return (
		<>
			<header>
				<h1>custody</h1>
				<p>
					The customer server, at <code>:8080</code>. It has no policy on it, so a headquarters
					credential typed below is served its own tenant's rows like anybody else's — there is
					nothing on that binary that answers wider.
				</p>
				<Signed hint="@acme/admin" who={who} onWho={setWho} />
				<p className="note">
					<code>Plain</code> believes what you type. It is what payday ships for development and it
					is not something to serve where anybody can reach it.
				</p>
			</header>

			<Opened name="customer" addr={ADDR} who={who}>
				<section>
					<h2>your assets</h2>
					<p>
						<code>AssetService.List</code> with no filter at all. The same request the headquarters
						page makes; the answer differs because the server narrows it.
					</p>
					<Assets />
				</section>

				<section>
					<h2>add one</h2>
					<p>
						The tenant it goes in is the one you are signed in as. Naming somebody else's is
						refused by the server, not by this page.
					</p>
					<Add tenant={tenantOf(who)} />
				</section>
			</Opened>

			<section>
				<h2>the public catalogue</h2>
				<p>
					Called with <strong>no credential</strong>, through a store of its own. What comes back is
					four fields — a different message, not an <code>Asset</code> with things blanked out — so
					nobody sees where a thing is or who is answerable for it.
				</p>
				<Anonymous>
					<Catalogue />
				</Anonymous>
			</section>
		</>
	)
}

createRoot(document.getElementById('root') as HTMLElement).render(<Customer />)
