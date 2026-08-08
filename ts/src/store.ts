/**
 * The local store and the reads that keep themselves up to date.
 *
 * `entities` is generated -- one declaration per entity, and no behaviour. The
 * runtime reads those together with the protobuf descriptors and implements
 * normalizing, reconciling and applying a `Watch` once, for every entity there
 * is. `Queries` is the half above it: a read that goes through it is one the
 * framework knows about, so a row that changes redraws every place it appears.
 *
 * Both pages read through this, and the interesting thing about that is what is
 * **not** in them: nothing declares which query a write invalidates, nothing
 * pushes a row into a list, and nothing tells the tenant beside one row that it
 * is the tenant beside another.
 *
 * @module
 */

import type { Transport } from '@connectrpc/connect'

import { Queries } from '@lesomnus/payday/query'
import type { App } from '@lesomnus/payday/react'
import { Store, identityOf } from '@lesomnus/payday/store'
import { openDisk } from '@lesomnus/payday/store/idb'

import { entities } from '../gen/entities.js'

export type { App }

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
 *
 * The two pages point at two servers, so `name` is what keeps those apart as
 * well: the same credential against `custody` and against `custody-admin` sees
 * different rows, and one mirror holding both would be a page drawing the other
 * server's answers.
 */
export async function open(name: string, transport: Transport, credential: string): Promise<App> {
	const at = { name, identity: await identityOf(credential) }

	const store = Store.open(entities, { ...at, disk: await openDisk(entities, at) })
	await store.hydrate()

	return { store, queries: new Queries(store, transport, entities) }
}
