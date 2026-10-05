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
  choiceDescriptions?: Record<string, string>;
  multiple: boolean;
  allowInput: boolean;
};

type CockpitAsk = {
  id: string;
  title: string | null;
  directory: string | null;
  createdAt: string;
  summary: string;
  choices: string[];
  choiceDescriptions?: Record<string, string>;
  multiple: boolean;
  allowInput?: boolean;
  questions?: CockpitQuestion[];
  media?: unknown[];
};

type CockpitResult<T> = { ok: true; data: T } | { ok: false; error: unknown };

type TerminalQuestion = {
  id: string;
  title: string;
  choices: string[];
  choiceDescriptions: string[];
  multiple: boolean;
  allowInput: boolean;
};

type TerminalAsk = {
  id: string;
  heading: string;
  time: string;
  summary: string;
  mediaCount: number;
  questions: TerminalQuestion[];
};

type AnswerEntry = { questionId: string | null; choiceIndexes: number[]; input: string };

type AnswerRequest = { answers: AnswerEntry[]; wholeAnswer: string };

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
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, "$1 ($2)")
    .replace(/\*\*|__|`/g, "")
    .replace(/^#{1,6}\s+/gm, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();

const questionsOf = (ask: CockpitAsk): CockpitQuestion[] =>
  ask.questions && ask.questions.length > 0
    ? ask.questions
    : [
        {
          id: "default",
          summary: ask.summary,
          choices: ask.choices,
          choiceDescriptions: ask.choiceDescriptions,
          multiple: ask.multiple === true,
          allowInput: ask.allowInput !== false,
        },
      ];

const toTerminalQuestion = ({ question, ask, questionCount }: { question: CockpitQuestion; ask: CockpitAsk; questionCount: number }): TerminalQuestion => ({
  id: question.id,
  title: questionCount > 1 || question.summary !== ask.summary ? toPlainText(question.summary) : "",
  choices: question.choices,
  choiceDescriptions: question.choices.map((choice) => question.choiceDescriptions?.[choice] ?? ""),
  multiple: question.multiple === true,
  allowInput: question.allowInput !== false,
});

const headingOf = (ask: CockpitAsk): string => {
  const title = ask.title || "確認リクエスト";
  const directoryLabel = ask.directory?.split(/[\\/]/).filter(Boolean).at(-1);
  return directoryLabel ? `${title} (${directoryLabel})` : title;
};

const timeOf = (createdAt: string): string => {
  const date = new Date(createdAt);
  const pad = (value: number): string => String(value).padStart(2, "0");
  return `${date.getMonth() + 1}/${date.getDate()} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
};

const byCreatedAt = (left: CockpitAsk, right: CockpitAsk): number =>
  left.createdAt.localeCompare(right.createdAt) || left.id.localeCompare(right.id);

const toTerminalAsk = (ask: CockpitAsk): TerminalAsk => {
  const questions = questionsOf(ask);
  return {
    id: ask.id,
    heading: headingOf(ask),
    time: timeOf(ask.createdAt),
    summary: toPlainText(ask.summary),
    mediaCount: ask.media?.length ?? 0,
    questions: questions.map((question) => toTerminalQuestion({ question, ask, questionCount: questions.length })),
  };
};

const entryArgs = (entry: AnswerEntry): string[] => [
  ...(entry.questionId === null ? [] : ["--question", entry.questionId]),
  ...entry.choiceIndexes.flatMap((index) => ["--choice-index", String(index)]),
  ...(entry.input === "" ? [] : ["--input", entry.input]),
];

const toAnswerArgs = ({ askId, request }: { askId: string; request: AnswerRequest }): string[] => [
  "ask",
  "answer",
  askId,
  ...request.answers.flatMap(entryArgs),
  ...(request.wholeAnswer === "" ? [] : ["--whole-answer", request.wholeAnswer]),
];

const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === "object" && value !== null;

const isAnswerEntry = (value: unknown): value is AnswerEntry =>
  isRecord(value) &&
  (value.questionId === null || typeof value.questionId === "string") &&
  Array.isArray(value.choiceIndexes) &&
  value.choiceIndexes.every((index: unknown) => Number.isInteger(index) && (index as number) >= 1) &&
  typeof value.input === "string" &&
  (value.choiceIndexes.length > 0 || value.input.trim() !== "");

const toAnswerRequest = (value: unknown): AnswerRequest | null => {
  if (!isRecord(value) || !Array.isArray(value.answers) || typeof value.wholeAnswer !== "string") return null;
  if (!value.answers.every(isAnswerEntry)) return null;
  const request: AnswerRequest = {
    answers: value.answers.map((entry) => ({ ...entry, input: entry.input.trim() })),
    wholeAnswer: value.wholeAnswer.trim(),
  };
  return request.answers.length > 0 || request.wholeAnswer !== "" ? request : null;
};

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

const relayCockpit = async ({ response, args }: { response: ServerResponse; args: string[] }): Promise<void> => {
  const result = await runCockpit<unknown>(args);
  if (!result.ok) return sendJson({ response, status: 502, body: { ok: false, error: result.error } });
  sendJson({ response, status: 200, body: { ok: true } });
};

const listAsks = async (response: ServerResponse): Promise<void> => {
  const result = await runCockpit<{ asks: CockpitAsk[] }>(["ask", "list"]);
  if (!result.ok) return sendJson({ response, status: 502, body: { ok: false, error: result.error } });
  sendJson({ response, status: 200, body: { ok: true, asks: [...result.data.asks].sort(byCreatedAt).map(toTerminalAsk) } });
};

const answerAsk = async ({ request, response, askId }: { request: IncomingMessage; response: ServerResponse; askId: string }): Promise<void> => {
  const answerRequest = toAnswerRequest(parseJson(await readBody(request)));
  if (!answerRequest) return sendJson({ response, status: 400, body: { ok: false, error: "invalid answers" } });
  await relayCockpit({ response, args: toAnswerArgs({ askId, request: answerRequest }) });
};

const route = async ({ request, response }: { request: IncomingMessage; response: ServerResponse }): Promise<void> => {
  const path = new URL(request.url ?? "/", "http://relay").pathname;
  if (request.method === "GET" && path === "/health") return sendJson({ response, status: 200, body: { ok: true } });
  if (!isAuthorized(request)) return sendJson({ response, status: 401, body: { ok: false, error: "unauthorized" } });
  if (request.method === "GET" && path === "/asks") return listAsks(response);
  const actionMatch = /^\/asks\/([A-Za-z0-9_-]+)\/(answer|close)$/.exec(path);
  if (request.method === "POST" && actionMatch?.[2] === "answer") return answerAsk({ request, response, askId: actionMatch[1] });
  if (request.method === "POST" && actionMatch?.[2] === "close") return relayCockpit({ response, args: ["ask", "close", actionMatch[1]] });
  sendJson({ response, status: 404, body: { ok: false, error: "not found" } });
};

createServer((request, response) => {
  route({ request, response }).catch((error: unknown) => sendJson({ response, status: 500, body: { ok: false, error: String(error) } }));
}).listen(config.port, "0.0.0.0", () => {
  console.log(`kobo-ask relay listening on :${config.port}`);
});
