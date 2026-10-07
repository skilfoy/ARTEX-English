#!/usr/bin/env python3
"""Unit tests for scripts/check_english.py. Run: python3 scripts/check_english_test.py"""

import unittest

from check_english import line_errors, scan_path

HAN = "\u4e2d\u6587"
NOTE = "\u266a"
NAME = "Je" + "an"
CALQUE = "It" + "'s not a token bucket"


class LineErrors(unittest.TestCase):
    def test_han_fails(self):
        self.assertIn("Han character", line_errors("label: " + HAN))

    def test_unicode_escape_is_not_han(self):
        self.assertEqual(line_errors('name := "\\u4eac ICP \\u5907"'), [])

    def test_music_note_fails(self):
        self.assertIn("music-note marker", line_errors("// " + NOTE + " fact"))

    def test_blocked_name_fails_as_a_word(self):
        self.assertIn("blocked name", line_errors("// " + NAME + " planner"))
        self.assertEqual(line_errors("jeans and Jeanette"), [])

    def test_comment_calque_fails(self):
        self.assertIn("comment calque", line_errors("// " + CALQUE))
        self.assertIn("comment calque", line_errors("/* " + "It" + "'s the credential */"))
        self.assertIn("comment calque", line_errors("# " + "It" + "'s a comment"))
        self.assertIn("comment calque", line_errors("-- " + "It" + "'s an sql note"))

    def test_prose_calque_outside_a_comment_is_kept(self):
        # Recorded transcripts are not comments. The calque check ignores them.
        prose = '"answer": "' + "It" + "'s a stand-alone question." + '"'
        self.assertEqual(line_errors(prose), [])

    def test_url_slashes_are_not_a_comment(self):
        prose = "see https://example.com " + "It" + "'s a path"
        self.assertEqual(line_errors(prose), [])


class ScanPath(unittest.TestCase):
    def test_allowed_fixture_shape_passes(self):
        content = 'icp := fmt.Sprintf("\\u4eac ICP \\u5907 %d \\u53f7", id)\n'
        self.assertEqual(scan_path("agent/blackboard_inheritance_test.go", content), [])

    def test_reports_the_line(self):
        self.assertEqual(scan_path("a.go", "// " + NAME + "\n"), ["a.go:1"])


if __name__ == "__main__":
    unittest.main()
