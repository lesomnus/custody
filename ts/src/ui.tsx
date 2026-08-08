/**
 * The parts both pages have, and nothing else.
 *
 * There is very little here on purpose. What these two pages are for is to show
 * that the generated client is the whole API and that the wall is on the
 * server, and anything that grows here is something a reader has to get past to
 * see that.
 *
 * @module
 */

import { useEffect, useState, type ReactNode } from 'react'

import { ConnectError } from '@connectrpc/connect'

import { Provider } from '@lesomnus/payday/react'

import { connect } from './client.js'
import { open, type App } from './store.js'

/**
 * Say puts an error on the page in the terms the server used.
 *
 * The code is worth showing rather than hiding behind "something went wrong":
 * `not_found` on a row somebody else holds is the wall doing its work, and
 * `permission_denied` from the admin server is that server saying it is not
 * yours -- two different things that a single message would flatten.
 */
export function Say(props: { err: unknown }): ReactNode {
	const e = ConnectError.from(props.err)

	return (
		<p className="err">
			<code>{e.code}</code> {e.rawMessage}
		</p>
	)
}

/** Table draws rows, and says so when there are none. */
export function Table(props: { head: string[]; children: ReactNode; empty: boolean }): ReactNode {
	if (props.empty) return <p className="empty">nothing here</p>

	return (
		<table>
			<thead>
				<tr>
					{props.head.map((h) => (
						<th key={h}>{h}</th>
					))}
				</tr>
			</thead>
			<tbody>{props.children}</tbody>
		</table>
	)
}

/** A timestamp as a person reads one, and empty when there is none. */
export function when(v: { seconds: bigint } | undefined): string {
	if (!v) return ''

	return new Date(Number(v.seconds) * 1000).toISOString().slice(0, 16).replace('T', ' ')
}

/**
 * Signed is the credential box, and where it is kept.
 *
 * `Plain` believes what is typed here. It is what payday ships for development
 * and it is not something to serve where anybody can reach it.
 */
export function Signed(props: { hint: string; who: string | null; onWho: (v: string | null) => void }): ReactNode {
	const [v, setV] = useState(props.who ?? '')

	return (
		<label>
			signed in as{' '}
			<input
				value={v}
				placeholder={props.hint}
				size={20}
				onChange={(e) => setV(e.target.value)}
				onBlur={() => props.onWho(v.trim() || null)}
				onKeyDown={(e) => {
					if (e.key === 'Enter') props.onWho(v.trim() || null)
				}}
			/>
		</label>
	)
}

/**
 * useOpened is one caller's store, opened and hydrated, and reopened when the
 * caller changes.
 *
 * Signing in as somebody else is a **different store**, not a store that is
 * emptied: what a caller may see is the actor and the scope together, so the
 * rows one credential drew have no business being on screen under another. The
 * old one is closed and the new one is filled from its own mirror, which is why
 * going back to a credential you used before is instant.
 */
export function useOpened(name: string, addr: string, who: string | null): App | undefined {
	const [app, setApp] = useState<App>()

	useEffect(() => {
		let live = true
		let got: App | undefined

		void open(name, connect(addr, () => who), who ?? '').then((v) => {
			if (!live) {
				v.store.close()

				return
			}

			got = v
			setApp(v)
		})

		return () => {
			live = false
			got?.store.close()
			setApp(undefined)
		}
	}, [name, addr, who])

	return app
}

/**
 * useCredential is who this page is signed in as, remembered between visits.
 *
 * Kept per page: the two point at two servers, and the credential that means
 * something on one is often not the one to use on the other.
 */
export function useCredential(name: string): [string | null, (v: string | null) => void] {
	const at = `custody.${name}.who`
	const [who, setWho] = useState<string | null>(() => localStorage.getItem(at))

	return [
		who,
		(v: string | null): void => {
			if (v === null) localStorage.removeItem(at)
			else localStorage.setItem(at, v)

			setWho(v)
		},
	]
}

/** Opened is the store for one caller, and the provider the tree reads through. */
export function Opened(props: {
	name: string
	addr: string
	who: string | null
	children: ReactNode
}): ReactNode {
	const app = useOpened(props.name, props.addr, props.who)
	if (app === undefined) return <p className="empty">opening...</p>

	return <Provider app={app}>{props.children}</Provider>
}
