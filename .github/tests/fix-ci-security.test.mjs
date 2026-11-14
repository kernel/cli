import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const workflow = readFileSync(new URL('../workflows/fix-ci.yaml', import.meta.url), 'utf8');
const jobs = Object.fromEntries(
  workflow
    .slice(workflow.indexOf('\njobs:\n'))
    .split(/^  (?=[\w-]+:\n)/m)
    .slice(1)
    .map((job) => [job.slice(0, job.indexOf(':')), job.split(/(?=^      - )/m)]),
);
const lock = { workflows: {}, dependencies: {} };
{
  let section;
  let key;
  for (const line of readFileSync(new URL('../workflows/actions.lock', import.meta.url), 'utf8').split('\n')) {
    let match;
    if ((match = line.match(/^(\w+):/))) {
      section = match[1];
    } else if ((match = line.match(/^    '([^']+)':/)) && section in lock) {
      key = match[1];
      lock[section][key] = section === 'workflows' ? [] : {};
    } else if (section === 'workflows' && (match = line.match(/^        - '([^']+)'$/))) {
      lock.workflows[key].push(match[1]);
    } else if (section === 'dependencies' && (match = line.match(/^        (\w+): '?([^']*)'?$/))) {
      lock.dependencies[key][match[1]] = match[2];
    }
  }
}
const header = (job) => jobs[job][0];
const steps = (job) => jobs[job].slice(1);
const step = (job, name) => {
  const value = steps(job).find((entry) => entry.startsWith(`      - name: ${name}\n`));
  assert.ok(value, `missing ${job} step: ${name}`);
  return value;
};
const order = (job, ...names) => {
  const indexes = names.map((name) => steps(job).indexOf(step(job, name)));
  assert.deepEqual(indexes, [...indexes].sort((a, b) => a - b), `${job} steps out of order: ${names}`);
};
const disallowedPaths = (paths) =>
  spawnSync('bash', [new URL('../scripts/fix-ci-disallowed-paths.sh', import.meta.url).pathname], {
    input: paths.map((path) => `${path}\0`).join(''),
    encoding: 'utf8',
  })
    .stdout.split('\0')
    .filter(Boolean);

test('only failed runs of same-repository PRs are handled', () => {
  const generate = header('generate');
  assert.match(generate, /github\.event\.workflow_run\.conclusion == 'failure'/);
  assert.match(generate, /github\.event\.workflow_run\.event == 'pull_request'/);
  assert.match(generate, /github\.event\.workflow_run\.head_repository\.full_name == github\.repository/);
  const find = step('generate', 'Find pull request');
  assert.match(find, /\.head\.repo\.full_name == env\.REPO/);
  assert.match(find, /\.head\.sha == env\.HEAD_SHA/);
  assert.match(header('publish'), /needs: generate\n/);
});

test('actions are pinned and checkouts do not persist credentials', () => {
  const locked = lock.workflows['.github/workflows/fix-ci.yaml'];
  assert.ok(locked, 'fix-ci.yaml is not onboarded in actions.lock');
  for (const entry of Object.keys(jobs).flatMap(steps).filter((entry) => entry.includes('uses: '))) {
    const uses = entry.match(/uses: (\S+)/)[1];
    if (uses.startsWith('$/')) continue;
    const match = uses.match(/^([\w.-]+\/[\w.-]+)(?:\/[\w./-]+)?@(\S+)$/);
    assert.ok(match, `unexpected action reference: ${uses}`);
    const [, repo, ref] = match;
    const dependency = lock.dependencies[`${repo}@${ref}`];
    assert.ok(locked.includes(`${repo}@${ref}`) && dependency, `${uses} is not locked in actions.lock`);
    if (/^[a-f0-9]{40}$/.test(ref)) {
      assert.equal(dependency.commit, `sha1-${ref}`);
      assert.doesNotMatch(dependency.ref, /^v\d+\.\d+\.\d+$/, `${uses} is a tagged release; use @${dependency.ref}`);
    } else {
      assert.match(ref, /^v\d+\.\d+\.\d+$/, `${uses} must use an exact vX.Y.Z release`);
      assert.equal(dependency.ref, ref);
    }
    if (entry.includes('uses: actions/checkout@')) {
      assert.match(entry, /persist-credentials: false/);
      assert.doesNotMatch(entry, /token:/);
    }
  }
});

test('expressions are not interpolated into shell scripts', () => {
  for (const entry of Object.keys(jobs).flatMap(steps).filter((entry) => entry.includes('run: |'))) {
    assert.doesNotMatch(entry.slice(entry.indexOf('run: |')), /\$\{\{/);
  }
});

test('the generate job has no write credentials', () => {
  assert.match(workflow, /^permissions:\n  contents: read\n\n/m);
  const generate = jobs.generate.join('');
  // id-token: write only lets the job mint an OIDC token for short-lived model credentials.
  assert.equal(generate.match(/: write\b/g).length, 1);
  assert.match(header('generate'), /^    permissions:\n(?:      [\w-]+: \w+\n)*      id-token: write\n/m);
  assert.doesNotMatch(generate, /create-github-app-token|ADMIN_APP/);
  assert.doesNotMatch(step('generate', 'Fix CI failure'), /GH_TOKEN:|GITHUB_TOKEN:|github\.token|outputs\.token/);
  // Agent and PR code run in this job, so caches it saves could reach other workflows.
  assert.match(step('generate', 'Setup Go'), /cache: false/);
});

test('Pi runs on the PR checkout with trusted instructions and no project resources', () => {
  const agent = step('generate', 'Fix CI failure');
  assert.match(agent, /working-directory: pr\n/);
  assert.match(agent, /< \.\.\/trusted\/\.github\/prompts\/fix-ci\.md/);
  assert.match(agent, /pi --print --no-session --no-approve /);
  assert.doesNotMatch(agent, /--approve\b/);
  assert.match(step('generate', 'Check out workflow files'), /path: trusted\n/);
  assert.doesNotMatch(step('generate', 'Check out workflow files'), /ref:/);
  assert.match(step('generate', 'Check out pull request'), /ref: \$\{\{ env\.HEAD_SHA \}\}/);
  order('generate', 'Check out workflow files', 'Find pull request', 'Record fix branch', 'Fix CI failure');
  const prompt = readFileSync(new URL('../prompts/fix-ci.md', import.meta.url), 'utf8');
  assert.match(prompt, /You have no GitHub credentials/);
  assert.match(prompt, /Do NOT commit, push, create PRs, or comment/);
});

test('changes are restricted to the allowlist and validated before upload', () => {
  order('generate', 'Fix CI failure', 'Snapshot publishable changes', 'Verify fix', 'Upload validated fix');
  const snapshot = step('generate', 'Snapshot publishable changes');
  assert.match(snapshot, /git reset -q --soft "\$HEAD_SHA"\n\s+git add -A/);
  assert.match(snapshot, /\| bash \.\.\/trusted\/\.github\/scripts\/fix-ci-disallowed-paths\.sh/);
  assert.match(snapshot, /git reset -q --hard\n\s+git clean -fdxq/);
  const verify = step('generate', 'Verify fix');
  assert.match(verify, /select\(\.New\.Version == null\)/);
  assert.match(verify, /100644\|100755\|000000/);
  assert.match(verify, /\n\s+make test\n/);
  assert.match(step('generate', 'Upload validated fix'), /if: steps\.snapshot\.outputs\.changed == 'true'/);
  const scriptCalls = workflow.match(/^.*disallowed-paths\.sh.*$/gm);
  assert.equal(scriptCalls.length, 2);
  for (const line of scriptCalls) assert.match(line, /\| bash \.\.\/trusted\//);
  assert.deepEqual(
    disallowedPaths([
      'go.mod',
      'go.sum',
      'cmd/browsers.go',
      'cmd/browsers_test.go',
      'cmd/proxies/check.go',
      'pkg/util/client.go',
      'pkg/templates/python/cua/_gitignore',
      '.github/workflows/test.yaml',
      '.github/scripts/fix-ci-disallowed-paths.sh',
      '.gitignore',
      '.cursor/commands/x.md',
      'pkg/templates/python/.python-version',
      'cmd/.hidden/x.go',
      'Makefile',
      'AGENTS.md',
      'scripts/release.sh',
      'bin/kernel',
      'go.work',
    ]),
    [
      '.github/workflows/test.yaml',
      '.github/scripts/fix-ci-disallowed-paths.sh',
      '.gitignore',
      '.cursor/commands/x.md',
      'pkg/templates/python/.python-version',
      'cmd/.hidden/x.go',
      'Makefile',
      'AGENTS.md',
      'scripts/release.sh',
      'bin/kernel',
      'go.work',
    ],
  );
});

test('publishing runs in a separate job that never executes repository code', () => {
  const publish = jobs.publish.join('');
  const runs = steps('publish')
    .filter((entry) => entry.includes('run: |'))
    .join('');
  assert.doesNotMatch(runs, /^\s*(go|make|npm|npx|node|pi|bash \.\/|\.\/)/m);
  assert.doesNotMatch(publish, /secrets\.(ANTHROPIC|OPENAI)_API_KEY|setup-go|setup-node/);
  const apply = step('publish', 'Apply validated fix');
  assert.match(apply, /test "\$\(git rev-parse HEAD\)" = "\$HEAD_SHA"/);
  assert.match(apply, /git apply --index "\$RUNNER_TEMP\/fix-ci\/fix-ci\.patch"/);
  assert.match(apply, /fix-ci-disallowed-paths\.sh/);
  assert.match(apply, /100644\|100755\|000000/);
  const mint = step('publish', 'Generate publishing token');
  assert.match(mint, /repositories: cli\n/);
  assert.match(mint, /permission-contents: write/);
  assert.match(mint, /permission-pull-requests: write/);
  assert.doesNotMatch(mint, /permission-workflows/);
  order('publish', 'Check out workflow files', 'Apply validated fix', 'Generate publishing token', 'Push fix and comment on PR');
  const beforeMint = steps('publish').slice(0, steps('publish').indexOf(mint)).join('');
  assert.doesNotMatch(beforeMint, /GH_TOKEN:/);
  const push = step('publish', 'Push fix and comment on PR');
  assert.match(push, /GH_TOKEN: \$\{\{ steps\.publish-token\.outputs\.token \}\}/);
  // Fixes go to a ci-fix branch for a human to propose, never to the PR branch.
  assert.match(push, /git push --force-with-lease="refs\/heads\/\$FIX_BRANCH:\$FIX_SHA" origin "HEAD:refs\/heads\/\$FIX_BRANCH"/);
  assert.equal(workflow.match(/git push/g).length, 1);
  assert.match(header('publish'), /FIX_BRANCH: ci-fix\/\$\{\{ github\.event\.workflow_run\.head_branch \}\}/);
  assert.doesNotMatch(publish, /gh pr (create|merge|edit)/);
  for (const entry of jobs.publish) {
    assert.doesNotMatch(entry, /continue-on-error: true|always\(\)/);
  }
});
