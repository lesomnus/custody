/**
 * The client, which is this small because nothing here is generated per
 * service.
 *
 * protobuf-es emits the service descriptors beside the messages and Connect's
 * `createClient` takes a descriptor, so adding an entity to the schema is one
 * line here and nothing to keep in step.
 *
 * @module
 */

import {
	createClient,
	type Client,
	type Interceptor,
	type Transport,
} from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'

import { AssetService } from '../gen/app/asset_svc_pb.js'
import { CatalogueService } from '../gen/app/catalogue_pb.js'
import { HolderService } from '../gen/payday/holder_svc_pb.js'
import { TenantService } from '../gen/payday/tenant_svc_pb.js'

export interface App {
	readonly asset: Client<typeof AssetService>
	readonly catalogue: Client<typeof CatalogueService>
	readonly tenant: Client<typeof TenantService>
	readonly holder: Client<typeof HolderService>
}

export function app(transport: Transport): App {
	return {
		asset: createClient(AssetService, transport),
		catalogue: createClient(CatalogueService, transport),
		tenant: createClient(TenantService, transport),
		holder: createClient(HolderService, transport),
	}
}

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
