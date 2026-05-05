import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { ArenaService } from "../gen/arena/v1/arena_pb";

const baseUrl = import.meta.env.VITE_ARENA_BASE_URL || window.location.origin;

const transport = createConnectTransport({ baseUrl });

export const arenaClient = createClient(ArenaService, transport);
