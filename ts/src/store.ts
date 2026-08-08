/**
 * The local store and the reads that keep themselves up to date.
 *
 * `entities` is generated -- one declaration per entity, and no behaviour. The
 * runtime reads those together with the protobuf descriptors and implements
 * normalizing, reconciling and applying a `Watch` once, for every entity there
 * is. `Queries` is the half above it: a read that goes through it is one the
 * framework knows about, so a row that changes redraws every place it appears.
 *
 * **The two pages do not use it.** They read the server on every render, which
 * is the right shape for two screens and the wrong one for an app somebody has
 * open all day. It is wired and type-checked here so that the next thing built
 * on this app has one line to write instead of a decision to make.
 *
 * @module
 */

import type { Transport } from '@connectrpc/connect'

import { Queries } from '@lesomnus/payday/query'
import { Store, identityOf } from '@lesomnus/payday/store'

import { entities } from '../gen/entities.js'

/** App is what a page would read through. */
export interface App {
	readonly store: Store
	readonly queries: Queries
}

/**
 * open answers with this app's store and queries, for one caller.
 *
 * Keyed on a digest of the **credential** rather than on a name for the person.
 * This app is the case that makes the difference concrete: a headquarters
 * operator holding a credential narrowed to one customer sees that customer,
 * and holding a whole one sees every tenant. Keyed on the person those two
 * share a store, and the narrowed session draws the wide session's rows --
 * nothing leaked, since the server sent all of it to that person, and the
 * screen is still wrong.
 */
export async function open(transport: Transport, credential: string): Promise<App> {
	const store = Store.open(entities, { name: 'custody', identity: await identityOf(credential) })

	return { store, queries: new Queries(store, transport, entities) }
}
