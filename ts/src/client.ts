/**
 * The transport, which is the only thing that changes between a real server and
 * a sandbox.
 *
 * A page does not read through a client -- it reads through `store.ts`, so that
 * a row it drew redraws when the row changes. This is what carries those calls,
 * and it is also what a script or a one-off would use `createClient` with
 * directly: protobuf-es emits the service descriptors beside the messages, so
 * there is nothing generated per service to keep in step.
 *
 * @module
 */

import { type Interceptor, type Transport } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'

/**
 * connect is a transport to a payday app's second listener.
 *
 * A browser cannot speak gRPC -- it is not a library that is missing, it is
 * frames the platform does not let a page write -- so what is on the other end
 * of this is `server.http` with `allow_web: true`, translating into the same
 * gRPC server every other client reaches.
 *
 * `who` is the credential, and it is `Plain`: the scheme that believes what the
 * caller writes. It is what payday ships for development and it must not be
 * served where anybody can reach it. Replacing it is `auth.Bearer` on the
 * server and a token here; nothing else in this file changes.
 */
export function connect(baseUrl: string, who: () => string | null): Transport {
	const credential: Interceptor = (next) => async (req) => {
		const v = who()
		if (v) {
			req.header.set('authorization', `Plain ${v}`)
		}

		return next(req)
	}

	return createConnectTransport({ baseUrl, interceptors: [credential] })
}
