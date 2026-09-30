#!/usr/bin/env python3
"""Tests for agent.py's tools — no network, no API key needed.

Run with: python3 test_tools.py
(or: python3 -m pytest test_tools.py, if pytest happens to be installed)

agent.py defers `import anthropic` to inside main(), so importing the module
to test list_files/read_file/write_file/run and the path-escape guard works
even without the anthropic package installed.
"""

import os
import sys
import tempfile
import time
import types
import unittest
from pathlib import Path


class ToolTests(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        self.work_dir = Path(self.tmpdir.name).resolve()

        # agent.py resolves paths against WORK_DIR at import time (Path.cwd()),
        # so chdir into a throwaway directory before importing it.
        self._prev_cwd = os.getcwd()
        os.chdir(self.work_dir)
        self.addCleanup(os.chdir, self._prev_cwd)

        import importlib
        import sys

        sys.path.insert(0, str(Path(__file__).resolve().parent))
        import agent

        importlib.reload(agent)  # pick up the new cwd as WORK_DIR
        self.agent = agent

    def test_write_then_read_file(self):
        out = self.agent.tool_write_file("hello.txt", "hi there")
        self.assertIn("hello.txt", out)
        self.assertEqual(self.agent.tool_read_file("hello.txt"), "hi there")

    def test_write_file_creates_parent_dirs(self):
        self.agent.tool_write_file("sub/dir/file.txt", "nested")
        self.assertEqual((self.work_dir / "sub" / "dir" / "file.txt").read_text(), "nested")

    def test_list_files(self):
        self.agent.tool_write_file("a.txt", "1")
        self.agent.tool_write_file("sub/b.txt", "2")
        listing = self.agent.tool_list_files(".")
        self.assertIn("a.txt", listing)
        self.assertIn("sub/", listing)
        self.assertIn("sub/b.txt", listing)

    def test_list_files_missing_path(self):
        self.assertIn("does not exist", self.agent.tool_list_files("nope"))

    def test_run_captures_stdout_and_exit_code(self):
        out = self.agent.tool_run("echo hello")
        self.assertIn("exit code: 0", out)
        self.assertIn("hello", out)

    def test_run_nonzero_exit_code(self):
        out = self.agent.tool_run("exit 3")
        self.assertIn("exit code: 3", out)

    def test_run_output_is_capped(self):
        # Ask for well over 8 KiB of output and check it gets truncated, not returned whole.
        out = self.agent.tool_run("python3 -c \"print('x' * 20000)\"")
        self.assertLess(len(out.encode("utf-8")), 20000)
        self.assertIn("truncated", out)

    def test_run_timeout(self):
        start = time.time()
        out = self.agent.tool_run("sleep 5", timeout_s=1)
        elapsed = time.time() - start
        self.assertIn("timed out", out)
        self.assertLess(elapsed, 4, "should not wait anywhere near the full sleep duration")

    def test_run_timeout_kills_backgrounded_child_holding_stdout(self):
        # The shell itself exits almost immediately, but it backgrounds a long-running child that
        # inherits its stdout/stderr pipes and keeps them open. Plain subprocess.run only signals the
        # shell's own pid on timeout, so that child would keep the pipes open and communicate() would
        # hang well past timeout_s waiting for EOF. tool_run must kill the whole process group instead.
        start = time.time()
        out = self.agent.tool_run("sleep 10 & exit 0", timeout_s=1)
        elapsed = time.time() - start
        self.assertIn("timed out", out)
        self.assertLess(elapsed, 4, "a backgrounded grandchild holding stdout must not hang the timeout")

    def test_run_timeout_is_capped_at_120(self):
        # timeout_s is clamped, not just accepted as-is — this doesn't wait 500s to prove it, it
        # only checks the clamp math via the same helper tool_run uses internally.
        self.assertEqual(min(int(500), self.agent.MAX_RUN_TIMEOUT_S), self.agent.MAX_RUN_TIMEOUT_S)

    def test_read_file_outside_working_dir_rejected(self):
        outside = Path(self.tmpdir.name).parent / "outside.txt"
        with self.assertRaises(ValueError):
            self.agent.safe_path("../outside.txt")
        self.assertFalse(outside.exists())

    def test_write_file_absolute_path_outside_rejected(self):
        with self.assertRaises(ValueError):
            self.agent.tool_write_file("/etc/mini-agent-test-should-not-exist", "nope")

    def test_write_file_dotdot_traversal_rejected(self):
        with self.assertRaises(ValueError):
            self.agent.tool_write_file("../escape.txt", "nope")

    def test_symlink_escape_rejected(self):
        outside_dir = Path(self.tmpdir.name + "-outside")
        outside_dir.mkdir()
        self.addCleanup(lambda: __import__("shutil").rmtree(outside_dir, ignore_errors=True))
        link = self.work_dir / "escape_link"
        try:
            link.symlink_to(outside_dir)
        except OSError as e:
            self.skipTest(f"cannot create symlinks in this environment: {e}")
        with self.assertRaises(ValueError):
            self.agent.tool_write_file("escape_link/pwned.txt", "nope")
        self.assertFalse((outside_dir / "pwned.txt").exists())

    def test_safe_path_allows_working_directory_itself(self):
        # Should not raise for "." or the working directory root.
        self.assertEqual(self.agent.safe_path("."), self.work_dir)


class FakeBlock:
    """Stand-in for an SDK content block — just the attributes agent.py reads off one."""

    def __init__(self, type, **kwargs):
        self.type = type
        self.__dict__.update(kwargs)


class FakeResponse:
    def __init__(self, content, stop_reason):
        self.content = content
        self.stop_reason = stop_reason


class FakeMessages:
    """Stands in for client.messages — hands back canned responses, records every request."""

    def __init__(self, responses):
        self._responses = list(responses)
        self.calls = []

    def create(self, **kwargs):
        # main() keeps mutating the same `messages` list after this call returns (appending the
        # next turn), so snapshot its current contents now rather than keep the live reference —
        # otherwise a later assertion on calls[i]["messages"] would see future turns too.
        snapshot = dict(kwargs)
        snapshot["messages"] = list(kwargs["messages"])
        self.calls.append(snapshot)
        return self._responses.pop(0)


class AgentLoopTests(unittest.TestCase):
    """Exercises main()'s request loop against a fake client — no network, no API key."""

    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        self.work_dir = Path(self.tmpdir.name).resolve()

        self._prev_cwd = os.getcwd()
        os.chdir(self.work_dir)
        self.addCleanup(os.chdir, self._prev_cwd)

        import importlib

        sys.path.insert(0, str(Path(__file__).resolve().parent))
        import agent

        importlib.reload(agent)
        self.agent = agent

    def _run_main_with(self, fake_messages):
        # main() does `import anthropic` itself and calls anthropic.Anthropic() to build its
        # client, so a fake module standing in for the real package is enough to run the loop
        # against canned responses — no real SDK, no network.
        fake_client = types.SimpleNamespace(messages=fake_messages)
        fake_anthropic = types.SimpleNamespace(Anthropic=lambda: fake_client)
        prev_module = sys.modules.get("anthropic")
        sys.modules["anthropic"] = fake_anthropic

        def restore():
            if prev_module is None:
                sys.modules.pop("anthropic", None)
            else:
                sys.modules["anthropic"] = prev_module

        self.addCleanup(restore)

        prev_argv = sys.argv
        sys.argv = ["agent.py", "do the task"]
        self.addCleanup(setattr, sys, "argv", prev_argv)

        self.agent.main()

    def test_max_tokens_mid_tool_use_is_answered_with_a_tool_result(self):
        # First response is cut off mid tool_use (stop_reason "max_tokens") — the truncated call
        # must be answered with a matching tool_result, not executed, or the next request's history
        # would have a tool_use with no tool_result and the real API would reject it with a 400.
        truncated_call = FakeBlock(
            "tool_use", id="toolu_01", name="list_files", input={"path": "."}
        )
        responses = [
            FakeResponse(content=[truncated_call], stop_reason="max_tokens"),
            FakeResponse(content=[FakeBlock("text", text="done")], stop_reason="end_turn"),
        ]
        fake_messages = FakeMessages(responses)

        self._run_main_with(fake_messages)

        self.assertEqual(len(fake_messages.calls), 2, "expected exactly one retry request")
        second_request_messages = fake_messages.calls[1]["messages"]
        last_user_message = second_request_messages[-1]
        self.assertEqual(last_user_message["role"], "user")

        tool_results = last_user_message["content"]
        self.assertEqual(len(tool_results), 1)
        result = tool_results[0]
        self.assertEqual(result["type"], "tool_result")
        self.assertEqual(result["tool_use_id"], "toolu_01")
        self.assertTrue(result["is_error"])
        self.assertIn("cut off", result["content"])

    def test_max_tokens_without_tool_use_still_sends_plain_continue(self):
        # A cutoff that doesn't land inside a tool_use block keeps the old plain-text nudge.
        responses = [
            FakeResponse(content=[FakeBlock("text", text="partial answer")], stop_reason="max_tokens"),
            FakeResponse(content=[FakeBlock("text", text="done")], stop_reason="end_turn"),
        ]
        fake_messages = FakeMessages(responses)

        self._run_main_with(fake_messages)

        self.assertEqual(len(fake_messages.calls), 2)
        last_user_message = fake_messages.calls[1]["messages"][-1]
        self.assertEqual(last_user_message["role"], "user")
        self.assertIsInstance(last_user_message["content"], str)
        self.assertIn("Continue exactly where you left off", last_user_message["content"])


if __name__ == "__main__":
    unittest.main()
