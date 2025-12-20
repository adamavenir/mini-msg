import { describe, it, expect } from 'vitest';
import {
  extractMentions,
  extractIssueRefs,
  matchesMention,
  isAllMention,
  highlightMentions,
} from '../src/core/mentions.ts';

describe('extractMentions', () => {
  it('should extract single mention', () => {
    expect(extractMentions('hey @alice')).toEqual(['alice']);
    expect(extractMentions('@bob how are you')).toEqual(['bob']);
    expect(extractMentions('what do you think @charlie')).toEqual(['charlie']);
  });

  it('should extract multiple mentions', () => {
    expect(extractMentions('hey @alice and @bob')).toEqual(['alice', 'bob']);
    expect(extractMentions('@alice @bob @charlie')).toEqual(['alice', 'bob', 'charlie']);
  });

  it('should extract full agent IDs', () => {
    expect(extractMentions('hey @alice.1')).toEqual(['alice.1']);
    expect(extractMentions('@alice.419 check this')).toEqual(['alice.419']);
    expect(extractMentions('@alice.frontend.3 please review')).toEqual(['alice.frontend.3']);
  });

  it('should extract complex agent IDs', () => {
    expect(extractMentions('@pm.5.sub.2 status update')).toEqual(['pm.5.sub.2']);
    expect(extractMentions('cc @alice.1 @pm.3.sub.1')).toEqual(['alice.1', 'pm.3.sub.1']);
  });

  it('should extract @all mention', () => {
    expect(extractMentions('@all heads up')).toEqual(['all']);
    expect(extractMentions('attention @all')).toEqual(['all']);
  });

  it('should handle @all with specific mentions', () => {
    expect(extractMentions('@all and especially @alice.1')).toEqual(['all', 'alice.1']);
  });

  it('should handle mentions with punctuation', () => {
    expect(extractMentions('@alice, please check')).toEqual(['alice']);
    expect(extractMentions('@alice: important!')).toEqual(['alice']);
    expect(extractMentions('@alice. @bob!')).toEqual(['alice', 'bob']);
    expect(extractMentions('@alice? @bob; @charlie.')).toEqual(['alice', 'bob', 'charlie']);
  });

  it('should handle mentions at start and end', () => {
    expect(extractMentions('@alice message here')).toEqual(['alice']);
    expect(extractMentions('message here @alice')).toEqual(['alice']);
    expect(extractMentions('@alice')).toEqual(['alice']);
  });

  it('should not extract from email addresses', () => {
    expect(extractMentions('contact@alice.com')).toEqual([]);
    expect(extractMentions('email me@bob.org')).toEqual([]);
  });

  it('should return empty array for no mentions', () => {
    expect(extractMentions('no mentions here')).toEqual([]);
    expect(extractMentions('just a regular message')).toEqual([]);
    expect(extractMentions('')).toEqual([]);
  });

  it('should not extract invalid mentions', () => {
    expect(extractMentions('@Alice')).toEqual([]); // uppercase
    expect(extractMentions('@1alice')).toEqual([]); // starts with number
    expect(extractMentions('@@alice')).toEqual(['alice']); // double @, should extract once
  });

  it('should handle mentions with alphanumeric segments', () => {
    expect(extractMentions('@alice2 hello')).toEqual(['alice2']);
    expect(extractMentions('@agent42.sub3.1')).toEqual(['agent42.sub3.1']);
  });

  it('should handle duplicate mentions', () => {
    expect(extractMentions('@alice and @alice again')).toEqual(['alice', 'alice']);
  });

  it('should handle prefixes and full IDs', () => {
    expect(extractMentions('@alice')).toEqual(['alice']);
    expect(extractMentions('@alice.1')).toEqual(['alice.1']);
    expect(extractMentions('@alice.frontend')).toEqual(['alice.frontend']);
  });
});

describe('matchesMention', () => {
  it('should match exact agent IDs', () => {
    expect(matchesMention('alice.1', 'alice.1')).toBe(true);
    expect(matchesMention('alice.419', 'alice.419')).toBe(true);
  });

  it('should match base prefixes', () => {
    expect(matchesMention('alice.1', 'alice')).toBe(true);
    expect(matchesMention('alice.419', 'alice')).toBe(true);
    expect(matchesMention('alice.frontend.3', 'alice')).toBe(true);
  });

  it('should match qualified prefixes', () => {
    expect(matchesMention('alice.frontend.3', 'alice.frontend')).toBe(true);
    expect(matchesMention('pm.5.sub.2', 'pm.5')).toBe(true);
    expect(matchesMention('pm.5.sub.2', 'pm.5.sub')).toBe(true);
  });

  it('should not match different bases', () => {
    expect(matchesMention('bob.1', 'alice')).toBe(false);
    expect(matchesMention('alice.frontend.1', 'bob')).toBe(false);
  });

  it('should not match partial segment names', () => {
    expect(matchesMention('alice.1', 'ali')).toBe(false);
    expect(matchesMention('alice.frontend.1', 'alice.front')).toBe(false);
  });
});

describe('isAllMention', () => {
  it('should identify @all mention', () => {
    expect(isAllMention('all')).toBe(true);
  });

  it('should reject non-@all mentions', () => {
    expect(isAllMention('alice')).toBe(false);
    expect(isAllMention('alice.1')).toBe(false);
    expect(isAllMention('All')).toBe(false);
    expect(isAllMention('ALL')).toBe(false);
    expect(isAllMention('')).toBe(false);
  });
});

describe('highlightMentions', () => {
  it('should highlight single mention', () => {
    expect(highlightMentions('hey @alice')).toBe('hey \x1b[36m@alice\x1b[0m');
    expect(highlightMentions('@bob check this')).toBe('\x1b[36m@bob\x1b[0m check this');
  });

  it('should highlight multiple mentions', () => {
    expect(highlightMentions('@alice and @bob')).toBe(
      '\x1b[36m@alice\x1b[0m and \x1b[36m@bob\x1b[0m'
    );
  });

  it('should highlight full agent IDs', () => {
    expect(highlightMentions('hey @alice.419')).toBe('hey \x1b[36m@alice.419\x1b[0m');
  });

  it('should highlight @all', () => {
    expect(highlightMentions('@all heads up')).toBe('\x1b[36m@all\x1b[0m heads up');
  });

  it('should not modify text without mentions', () => {
    expect(highlightMentions('no mentions here')).toBe('no mentions here');
  });

  it('should not highlight email addresses', () => {
    expect(highlightMentions('contact@alice.com')).toBe('contact@alice.com');
  });

  it('should respect NO_COLOR environment variable', () => {
    const originalNoColor = process.env.NO_COLOR;

    // Test with NO_COLOR set
    process.env.NO_COLOR = '1';
    expect(highlightMentions('hey @alice')).toBe('hey @alice');
    expect(highlightMentions('@alice and @bob')).toBe('@alice and @bob');
    expect(highlightMentions('@all heads up')).toBe('@all heads up');

    // Test with NO_COLOR unset
    delete process.env.NO_COLOR;
    expect(highlightMentions('hey @alice')).toBe('hey \x1b[36m@alice\x1b[0m');

    // Restore original value
    if (originalNoColor !== undefined) {
      process.env.NO_COLOR = originalNoColor;
    } else {
      delete process.env.NO_COLOR;
    }
  });
});

describe('extractIssueRefs', () => {
  it('should extract single issue ref', () => {
    expect(extractIssueRefs('@bdm-123 hello')).toEqual(['bdm-123']);
    expect(extractIssueRefs('see @api-456 for details')).toEqual(['api-456']);
    expect(extractIssueRefs('fixed in @proj-0z5')).toEqual(['proj-0z5']);
  });

  it('should extract multiple issue refs', () => {
    expect(extractIssueRefs('@bdm-123 and @api-456')).toEqual(['bdm-123', 'api-456']);
    expect(extractIssueRefs('@proj-1 @proj-2 @proj-3')).toEqual(['proj-1', 'proj-2', 'proj-3']);
  });

  it('should ignore agent mentions', () => {
    expect(extractIssueRefs('@alice.1 hello')).toEqual([]);
    expect(extractIssueRefs('@bob check this')).toEqual([]);
    expect(extractIssueRefs('@all heads up')).toEqual([]);
  });

  it('should handle mixed mentions and issue refs', () => {
    expect(extractIssueRefs('@alice.1 @bdm-123')).toEqual(['bdm-123']);
    expect(extractIssueRefs('@bdm-123 cc @bob.2')).toEqual(['bdm-123']);
    expect(extractIssueRefs('@all see @bdm-123 and @api-456')).toEqual(['bdm-123', 'api-456']);
  });

  it('should deduplicate issue refs', () => {
    expect(extractIssueRefs('@bdm-123 @bdm-123')).toEqual(['bdm-123']);
    expect(extractIssueRefs('see @api-1 and @api-1 again')).toEqual(['api-1']);
  });

  it('should handle issue refs with punctuation', () => {
    expect(extractIssueRefs('@bdm-123, @api-456.')).toEqual(['bdm-123', 'api-456']);
    expect(extractIssueRefs('@proj-1: important!')).toEqual(['proj-1']);
    expect(extractIssueRefs('@web-99?')).toEqual(['web-99']);
  });

  it('should handle alphanumeric issue IDs', () => {
    expect(extractIssueRefs('@bdm-abc')).toEqual(['bdm-abc']);
    expect(extractIssueRefs('@proj-0z5')).toEqual(['proj-0z5']);
    expect(extractIssueRefs('@api-123abc')).toEqual(['api-123abc']);
  });

  it('should return empty array for no issue refs', () => {
    expect(extractIssueRefs('no refs here')).toEqual([]);
    expect(extractIssueRefs('@alice.1 @bob.2')).toEqual([]);
    expect(extractIssueRefs('')).toEqual([]);
  });

  it('should not match invalid patterns', () => {
    expect(extractIssueRefs('@123-abc')).toEqual([]);
    expect(extractIssueRefs('@Bdm-123')).toEqual([]);
    expect(extractIssueRefs('@bdm--123')).toEqual([]);
  });

  it('should handle issue refs at start and end', () => {
    expect(extractIssueRefs('@bdm-123 message here')).toEqual(['bdm-123']);
    expect(extractIssueRefs('message here @bdm-123')).toEqual(['bdm-123']);
    expect(extractIssueRefs('@bdm-123')).toEqual(['bdm-123']);
  });

  it('should normalize to lowercase', () => {
    expect(extractIssueRefs('@bdm-123')).toEqual(['bdm-123']);
    expect(extractIssueRefs('@api-ABC')).toEqual(['api-abc']);
  });
});
