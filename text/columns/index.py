import re
from sys import argv

##1 ^^^^^ ###1 @@0 cnt=3 poolMinBaseIdx=57 startBaseIdx=61 pool=[245.42 - 245.42] 1 words: "1"

regex = re.compile(r'###1 @@0\s+cnt=(\d+)\s+poolMinBaseIdx=(\d+)\s+startBaseIdx=(\d+)\s+pool=\[(\S+)\s*\-\s*(\S+)\]')
regex = re.compile(r'###1 @@0\s+cnt=(\d+)\s+poolMinBaseIdx=(\d+)\s+startBaseIdx=(\d+)')

line = '^^^^^ ###1 @@0 cnt=3 poolMinBaseIdx=57 startBaseIdx=61 pool=[245.42 - 245.42] 1 words: "1"'
m = regex.search(line)
assert m


def scan(path):
	nLines = 0
	matches = []
	with open(path, 'rt', errors='ignore') as f:
		for line in f:
			line = line[:-1].strip()
			nLines += 1
			m = regex.search(line)
			if not m:
				continue
			# minBase = float(m.group(4))
			# maxBase = float(m.group(5))
			minBase = 0
			maxBase = 0
			cnt = int(m.group(1))
			poolIdx = int(m.group(2))
			startIdx = int(m.group(3))
			matches.append((nLines, cnt, poolIdx, startIdx, minBase, maxBase, line))
	return nLines, matches

_, matches1 = scan(argv[1])
_, matches2 = scan(argv[2])
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))

n = min( len(matches1),  len(matches2))
badI = -1
print('        %-15s    %s'% (argv[1], argv[2]))
for i in range(n):
	n1, c1, p1, s1 ,i1, a1, l1 = matches1[i]
	n2, c2, p2, s2, i2, a2, l2 = matches2[i]
	bad = c1 != c2 or p1 != p2 or s1 != s2
	marker = '******' if bad else ''
	print('%3d: (%3d %3d %3d)  %3d %3d %3d) %s' % (i,
         c1, p1, s1, c2, p2, s2, marker))
	if bad:
		print('%6d: %s' % (n1, l1))
		print('%6d: %s' % (n2, l2))
	if badI < 0:
		if bad:
			badI = i
	elif i > badI + 5:
		break

