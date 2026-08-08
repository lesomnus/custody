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

import { useState } from 'react'
import { createRoot } from 'react-dom/client'

import { useCall, useQuery } from '@lesomnus/payday/react'
import { key } from '@lesomnus/payday/store'

import { AssetService } from '../gen/app/asset_svc_pb.js'
import { TenantService } from '../gen/payday/tenant_svc_pb.js'
import { Opened, Say, Signed, Table, useCredential } from './ui.js'
import './style.css'

/** Where the headquarters server is; see `custody-admin.yaml`. */
const ADDR = 'http://localhost:8081'

/**
 * Assets is every tenant's, and the request says nothing about tenants.
 *
 * `Hq.Where` answers "every tenant" and the credential is met with it, so an
 * operator whose credential names one customer sees that customer and nobody
 * else -- from this same page, with this same call. Sign in as a customer's
 * holder and the server refuses outright: `permission_denied`, from `Hq.May`,
 * before a row is read.
 */
function Assets(): React.ReactNode {
	const { data, error } = useQuery(AssetService.method.list, {})

	if (error !== undefined) return <Say err={error} />
	if (data === undefined) return <p className="empty">...</p>

	return (
		<Table head={['asset', 'name', 'where', 'tenant']} empty={data.items.length === 0}>
			{data.items.map((v) => (
				<tr key={key(v.id)}>
					<td>{v.alias}</td>
					<td>{v.name}</td>
					<td>{v.location}</td>
					<td>
						<Tenant id={v.tenant?.id} />
					</td>
				</tr>
			))}
		</Table>
	)
}

/**
 * Tenant is the name of the tenant a row is in, which the list did not carry.
 *
 * A `List` answers with rows and the **keys** of their edges: `tenant` comes
 * back with an identifier in it and nothing else, because the server did not
 * join what nobody asked for. So the name is a read, and this is where it is
 * asked for -- from the component that shows it, rather than from a loop in the
 * page that has to be kept in step with the table.
 *
 * Which is affordable because the read goes through the framework: two rows in
 * one tenant is **one** call, since a query is its method and its request and
 * these two are the same question. Twenty assets across three tenants is three.
 *
 * The customer page has none of this: every row it can see is in the tenant it
 * is signed in as.
 */
function Tenant(props: { id: Uint8Array | undefined }): React.ReactNode {
	const { data } = useQuery(
		TenantService.method.get,
		{ ref: { key: { case: 'id', value: props.id ?? new Uint8Array() } } },
		// No `Watch` on a tenant's name: it is read to be shown beside a row and
		// a stream for each of them is a stream per tenant on screen.
		{ watch: false },
	)

	return <code>{data?.alias ?? '?'}</code>
}

/**
 * Transfer moves an asset to another tenant.
 *
 * It is an RPC written by hand rather than a `Patch`, and the reason is in
 * `proto/ext/app/asset_svc.ext.proto`: `Asset.tenant` is mutable, so a general
 * write could set it, which is exactly why the general writes are closed. The
 * rule -- a reason somebody can read in a year, a destination the caller can
 * see -- lives in the one door.
 *
 * What it answers with is the asset, so the row in the table above moves to its
 * new tenant without this form telling it to.
 */
function Transfer(): React.ReactNode {
	const [v, setV] = useState({ alias: '', from: '', to: '', reason: '' })
	const transfer = useCall(AssetService.method.transfer)

	return (
		<>
			<form
				className="form"
				onSubmit={(e) => {
					e.preventDefault()
					transfer
						.call({
							ref: {
								key: {
									case: 'slug',
									value: { alias: v.alias, tenant: { key: { case: 'alias', value: v.from } } },
								},
							},
							to: { key: { case: 'alias', value: v.to } },
							reason: v.reason,
						})
						.then(() => setV({ ...v, alias: '', reason: '' }))
						.catch(() => {})
				}}
			>
				<input
					value={v.alias}
					placeholder="asset number"
					size={16}
					onChange={(e) => setV({ ...v, alias: e.target.value })}
				/>
				<input
					value={v.from}
					placeholder="from tenant"
					size={12}
					onChange={(e) => setV({ ...v, from: e.target.value })}
				/>
				<input
					value={v.to}
					placeholder="to tenant"
					size={12}
					onChange={(e) => setV({ ...v, to: e.target.value })}
				/>
				<input
					value={v.reason}
					placeholder="why, in a sentence"
					size={36}
					onChange={(e) => setV({ ...v, reason: e.target.value })}
				/>
				<button type="submit" disabled={transfer.state === 'pending'}>
					transfer
				</button>
			</form>
			{transfer.state === 'error' && <Say err={transfer.error} />}
			{transfer.state === 'ok' && transfer.data !== undefined && (
				<p className="ok">
					{transfer.data.alias} is now <Tenant id={transfer.data.tenant?.id} />
					&apos;s
				</p>
			)}
		</>
	)
}

function Admin(): React.ReactNode {
	const [who, setWho] = useCredential('admin')

	return (
		<>
			<header>
				<h1>
					custody <span className="tag">headquarters</span>
				</h1>
				<p>
					The admin server, at <code>:8081</code>, which is deployed where only the company can
					reach it. The calls on this page are the same calls the customer page makes; the
					difference is that this binary was handed a policy and that one was not.
				</p>
				<Signed hint="@hq/admin" who={who} onWho={setWho} />
				<p className="note">
					Sign in as a customer&apos;s holder to see <code>Hq.May</code> refuse you outright — this
					server is for headquarters, and it says so before a row is read.
				</p>
			</header>

			<Opened name="admin" addr={ADDR} who={who}>
				<section>
					<h2>every tenant&apos;s assets</h2>
					<p>
						<code>AssetService.List</code>, no filter, no mention of a tenant. An operator whose
						credential names one customer sees that customer from this same page: the policy
						answers &quot;every tenant&quot; and the credential is met with it.
					</p>
					<Assets />
				</section>

				<section>
					<h2>transfer</h2>
					<p>
						An RPC of its own, because <code>Asset.tenant</code> is mutable and a{' '}
						<code>Patch</code> could set it — which is why the general writes are closed. The
						reason is required: the trail stays with the tenant the asset leaves.
					</p>
					<Transfer />
				</section>
			</Opened>
		</>
	)
}

createRoot(document.getElementById('root') as HTMLElement).render(<Admin />)
