/**
 * The local store: a replica of what this caller may see.
 *
 * `entities` is generated -- one declaration per entity, and no behaviour. The
 * store reads those together with the protobuf descriptors and implements
 * normalizing, reconciling and applying a `Watch` once, for every entity there
 * is.
 *
 * **The two pages do not use it.** They call the server on every read, which is
 * the right shape for two screens and the wrong one for an app somebody has
 * open all day: a store is what turns `Watch` into a page that is already
 * correct when it renders. It is here, wired and type-checked, so that the
 * next thing built on this app has one line to write instead of a decision to
 * make.
 *
 * @module
 */

import { Store } from '@lesomnus/payday/store'

import { entities } from '../gen/entities.js'

/**
 * open answers with this app's store for one caller.
 *
 * The identity is not optional. A store shared between two callers shows the
 * first one's rows to the second -- nothing leaked, since the server never sent
 * anything they could not see, but the screen is wrong.
 */
export function open(identity: string): Store {
	return Store.open(entities, { name: 'custody', identity })
}
