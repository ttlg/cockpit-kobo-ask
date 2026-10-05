import { execFile } from "node:child_process";
import { randomBytes, timingSafeEqual } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { homedir } from "node:os";
import { join } from "node:path";

type CockpitQuestion = {
  id: string;
  summary: string;
  choices: string[];
  multiple: boolean;
};

type CockpitAsk = {
  id: string;
  title?: string;
  summary: string;
  choices: string[];
  multiple: boolean;
  questions?: CockpitQuestion[];
  media?: unknown[];
};

type CockpitResult<T> = { ok: true; data: T } | { ok: false; error: unknown };

type TerminalQuestion = {
  id: string | null;
  summary: string;
  choices: string[];
  multiple: boolean;
};

type TerminalAsk = {
  id: string;
  title: string;
  summary: string;
  answerable: boolean;
  unanswerableReason: string | null;
  questions: TerminalQuestion[];
};

type AnswerGroup = { questionId: string | null; choiceIndexes: number[] };

type RelayConfig = { port: number; token: string };

const configPath = join(import.meta.dirname, "config.json");
const cockpitBin = process.env.COCKPIT_BIN ?? join(homedir(), ".agi-tools", "bin", "cockpit");

const loadConfig = (): RelayConfig => {
  if (existsSync(configPath)) return JSON.parse(readFileSync(configPath, "utf8")) as RelayConfig;
  const created: RelayConfig = { port: 47390, token: randomBytes(24).toString("base64url") };
  writeFileSync(configPath, `${JSON.stringify(created, null, 2)}\n`, { mode: 0o600 });
  return created;
};

const config = loadConfig();

const runCockpit = <T>(args: string[]): Promise<CockpitResult<T>> =>
  new Promise((resolve) => {
    execFile(cockpitBin, ["--channel", "production", ...args], { timeout: 20_000, maxBuffer: 16 * 1024 * 1024 }, (_error, stdout) => {
      const parsed = (() => {
        try {
          return JSON.parse(stdout) as CockpitResult<T>;
        } catch {
          return { ok: false, error: stdout.trim() || "cockpit returned no output" } as const;
        }
      })();
      resolve(parsed);
    });
  });

const toPlainText = (markdown: string): string =>
  markdown
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/\*\*|__|`/g, "")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();

const toTerminalQuestions = (ask: CockpitAsk): TerminalQuestion[] =>
  ask.questions && ask.questions.length > 0
    ? ask.questions.map((question) => ({
        id: question.id,
        summary: toPlainText(question.summary),
        choices: question.choices,
        multiple: question.multiple,
      }))
    : [{ id: null, summary: "", choices: ask.choices, multiple: ask.multiple }];

const unanswerableReasonOf = ({ ask, questions }: { ask: CockpitAsk; questions: TerminalQuestion[] }): string | null => {
  if (ask.media && ask.media.length > 0) return "画像や動画が付いているので、Mac で確認して回答してください。";
  if (questions.some((question) => question.choices.length === 0)) return "自由入力の質問なので、Mac で回答してください。";
  return null;
};

const toTerminalAsk = (ask: CockpitAsk): TerminalAsk => {
  const questions = toTerminalQuestions(ask);
  const unanswerableReason = unanswerableReasonOf({ ask, questions });
  return {
    id: ask.id,
    title: ask.title ?? "",
    summary: toPlainText(ask.summary),
    answerable: unanswerableReason === null,
    unanswerableReason,
    questions,
  };
};

const toAnswerArgs = ({ askId, answers }: { askId: string; answers: AnswerGroup[] }): string[] => [
  "ask",
  "answer",
  askId,
  ...answers.flatMap((group) => [
    ...(group.questionId === null ? [] : ["--question", group.questionId]),
    ...group.choiceIndexes.flatMap((index) => ["--choice-index", String(index)]),
  ]),
];

const isAnswerGroups = (value: unknown): value is AnswerGroup[] =>
  Array.isArray(value) &&
  value.length > 0 &&
  value.every(
    (group: unknown) =>
      typeof group === "object" &&
      group !== null &&
      "questionId" in group &&
      (group.questionId === null || typeof group.questionId === "string") &&
      "choiceIndexes" in group &&
      Array.isArray(group.choiceIndexes) &&
      group.choiceIndexes.length > 0 &&
      group.choiceIndexes.every((index: unknown) => Number.isInteger(index) && (index as number) >= 1),
  );

const sendJson = ({ response, status, body }: { response: ServerResponse; status: number; body: unknown }): void => {
  response.writeHead(status, { "Content-Type": "application/json; charset=utf-8" });
  response.end(JSON.stringify(body));
};

const isAuthorized = (request: IncomingMessage): boolean => {
  const expected = Buffer.from(`Bearer ${config.token}`);
  const actual = Buffer.from(request.headers.authorization ?? "");
  return actual.length === expected.length && timingSafeEqual(actual, expected);
};

const readBody = (request: IncomingMessage): Promise<string> =>
  new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    request.on("data", (chunk: Buffer) => chunks.push(chunk));
    request.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    request.on("error", reject);
  });

const parseJson = (text: string): unknown => {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
};

const listAsks = async (response: ServerResponse): Promise<void> => {
  const result = await runCockpit<{ asks: CockpitAsk[] }>(["ask", "list"]);
  if (!result.ok) return sendJson({ response, status: 502, body: { ok: false, error: result.error } });
  sendJson({ response, status: 200, body: { ok: true, asks: result.data.asks.map(toTerminalAsk) } });
};

const answerAsk = async ({ request, response, askId }: { request: IncomingMessage; response: ServerResponse; askId: string }): Promise<void> => {
  const body = parseJson(await readBody(request));
  const answers = typeof body === "object" && body !== null && "answers" in body ? body.answers : null;
  if (!isAnswerGroups(answers)) return sendJson({ response, status: 400, body: { ok: false, error: "invalid answers" } });
  const result = await runCockpit<unknown>(toAnswerArgs({ askId, answers }));
  if (!result.ok) return sendJson({ response, status: 502, body: { ok: false, error: result.error } });
  sendJson({ response, status: 200, body: { ok: true } });
};

const route = async ({ request, response }: { request: IncomingMessage; response: ServerResponse }): Promise<void> => {
  const path = new URL(request.url ?? "/", "http://relay").pathname;
  if (request.method === "GET" && path === "/health") return sendJson({ response, status: 200, body: { ok: true } });
  if (!isAuthorized(request)) return sendJson({ response, status: 401, body: { ok: false, error: "unauthorized" } });
  if (request.method === "GET" && path === "/asks") return listAsks(response);
  const answerMatch = /^\/asks\/([A-Za-z0-9_-]+)\/answer$/.exec(path);
  if (request.method === "POST" && answerMatch) return answerAsk({ request, response, askId: answerMatch[1] });
  sendJson({ response, status: 404, body: { ok: false, error: "not found" } });
};

createServer((request, response) => {
  route({ request, response }).catch((error: unknown) => sendJson({ response, status: 500, body: { ok: false, error: String(error) } }));
}).listen(config.port, "0.0.0.0", () => {
  console.log(`kobo-ask relay listening on :${config.port}`);
});
