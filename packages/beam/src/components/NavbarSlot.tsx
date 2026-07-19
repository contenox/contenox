import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type ReactNode,
  type SetStateAction,
} from 'react';

/**
 * A single-occupant "slot" the app shell exposes in the CENTER of its top navbar,
 * so a routed page can project one piece of chrome into the global navbar
 * instead of spending vertical space on its own header strip. Today the chat
 * surface fills it with its connection badge (see `ChatConnectionBadge`), letting
 * `AcpChatPage` drop the ~56px header row it used to render.
 *
 * Single-owner by construction: the LAST component to mount a `useNavbarSlot(...)`
 * wins the slot, and a component clears the slot when it unmounts — but ONLY if
 * it still holds its own node (see `releaseNavbarSlot`), so a page tearing down
 * AFTER its successor already claimed the slot can't stomp the newcomer. In
 * practice only one route ever fills it at a time; the guard just makes the
 * mount/unmount ordering race-proof.
 *
 * The setter and the value live in SEPARATE contexts on purpose: `useNavbarSlot`
 * consumers read only the (stable) setter, so pushing a new node re-renders the
 * navbar that reads the value — never the page that supplied it. That split is
 * what keeps `useNavbarSlot(<Foo/>)` from looping when the supplying page
 * re-renders with a fresh element each time.
 */

type SetNavbarSlotNode = Dispatch<SetStateAction<ReactNode>>;

const NavbarSlotValueContext = createContext<ReactNode>(null);
const NavbarSlotSetterContext = createContext<SetNavbarSlotNode>(() => {});

export function NavbarSlotProvider({ children }: { children: ReactNode }) {
  const [node, setNode] = useState<ReactNode>(null);
  return (
    <NavbarSlotSetterContext.Provider value={setNode}>
      <NavbarSlotValueContext.Provider value={node}>{children}</NavbarSlotValueContext.Provider>
    </NavbarSlotSetterContext.Provider>
  );
}

/** The node currently occupying the navbar slot (or `null`). Read by the shell's navbar. */
export function useNavbarSlotValue(): ReactNode {
  return useContext(NavbarSlotValueContext);
}

/**
 * Pure single-owner release: on unmount a claimant clears the slot ONLY if the
 * slot still holds the exact node it put there. If a later mounter has since
 * taken over, `current !== claimed`, so this stale release leaves the newcomer's
 * node untouched. Pure so the "last mounter wins / cleared when it leaves"
 * contract is unit-testable without a DOM (see `NavbarSlot.test.ts`).
 */
export function releaseNavbarSlot(current: ReactNode, claimed: ReactNode): ReactNode {
  return current === claimed ? null : current;
}

/**
 * Projects `node` into the navbar slot for as long as the calling component is
 * mounted (last mounter wins), clearing it on unmount. `node` is captured at
 * mount, so pass a self-contained element — one that reads its own context and
 * takes no props, like `<ChatConnectionBadge/>` — which stays correct as its own
 * state changes without this hook re-pushing it on every render.
 */
export function useNavbarSlot(node: ReactNode): void {
  const setNode = useContext(NavbarSlotSetterContext);
  const nodeRef = useRef(node);
  nodeRef.current = node;
  useEffect(() => {
    const claimed = nodeRef.current;
    setNode(claimed);
    return () => setNode(current => releaseNavbarSlot(current, claimed));
  }, [setNode]);
}
