import { initTRPC } from "@trpc/server";
import { z } from "zod";

const t = initTRPC.create();

export const router = t.router;
export const publicProcedure = t.procedure;

export const appRouter = router({
    health: publicProcedure.query(() => {
        return { status: "ok" };
    }),

    listAgents: publicProcedure
        .input(z.object({ namespace: z.string().optional() }))
        .query(async ({ input }) => {
            // TODO: Connect to K8s API to list Agent CRs
            return { agents: [], namespace: input.namespace ?? "default" };
        }),

    listSkills: publicProcedure
        .input(z.object({ namespace: z.string().optional() }))
        .query(async ({ input }) => {
            // TODO: Connect to K8s API to list Skill CRs
            return { skills: [], namespace: input.namespace ?? "default" };
        }),
});

export type AppRouter = typeof appRouter;
