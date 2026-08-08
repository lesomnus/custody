/**
 * Enough DOM to draw a table and say what went wrong, and nothing else.
 *
 * There is no framework here on purpose. What these two pages are for is to
 * show that the generated client is the whole API -- that a screen is a call
 * and a call is the same call a `grpcurl` makes -- and a framework would be the
 * largest thing in the repository saying nothing about that.
 *
 * @module
 */

import { ConnectError } from '@connectrpc/connect'

export function el<K extends keyof HTMLElementTagNameMap>(
	tag: K,
	attrs: Record<string, string> = {},
	...children: (Node | string)[]
): HTMLElementTagNameMap[K] {
	const v = document.createElement(tag)
	for (const [k, x] of Object.entries(attrs)) {
		v.setAttribute(k, x)
	}
	v.append(...children)

	return v
}

export function mount(id: string): HTMLElement {
	const v = document.getElementById(id)
	if (!v) {
		throw new Error(`no #${id} in this page`)
	}

	return v
}

/** table draws rows, and says so when there are none. */
export function table(head: string[], rows: (Node | string)[][]): HTMLElement {
	if (rows.length === 0) {
		return el('p', { class: 'empty' }, 'nothing here')
	}

	return el(
		'table',
		{},
		el('thead', {}, el('tr', {}, ...head.map((h) => el('th', {}, h)))),
		el(
			'tbody',
			{},
			...rows.map((r) => el('tr', {}, ...r.map((c) => el('td', {}, c)))),
		),
	)
}

/**
 * say puts an error on the page in the terms the server used.
 *
 * The code is worth showing rather than hiding behind "something went wrong":
 * `not_found` on a row somebody else holds is the wall doing its work, and
 * `permission_denied` from the admin server is that server saying it is not
 * yours -- two different things that a single message would flatten.
 */
export function say(at: HTMLElement, err: unknown): void {
	const e = ConnectError.from(err)
	at.replaceChildren(
		el('p', { class: 'err' }, el('code', {}, ConnectError.name), ' '),
		el('p', { class: 'err' }, `${e.code}: ${e.rawMessage}`),
	)
}

/** run calls `f` and draws whatever it answers with, or whatever it refuses with. */
export async function run(at: HTMLElement, f: () => Promise<Node>): Promise<void> {
	try {
		at.replaceChildren(await f())
	} catch (err) {
		say(at, err)
	}
}

/** A timestamp as a person reads one, and empty when there is none. */
export function when(v: { seconds: bigint } | undefined): string {
	if (!v) {
		return ''
	}

	return new Date(Number(v.seconds) * 1000).toISOString().slice(0, 16).replace('T', ' ')
}
