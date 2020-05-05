import re
from sys import argv

##1 [INFO]  text.go:1404 ###1 Adding words to block cnt=0 dnt=0 minBase=99.96 maxBase=99.96

regex = re.compile(r'###1 Adding words to block\s+cnt=(\d+)\s+dnt=(\d+)\s+minBase=(\S+)\s+maxBase=(\S+)')
line =              '###1 Adding words to block cnt=0 dnt=0 minBase=99.96 maxBase=99.96'
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
			minBase = float(m.group(3))
			maxBase = float(m.group(3))
			cnt = int(m.group(1))
			dnt = int(m.group(2))
			matches.append((nLines, minBase, maxBase, cnt, dnt, line))
	return nLines, matches

_, matches1 = scan(argv[1])
_, matches2 = scan(argv[2])
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))

n = min( len(matches1),  len(matches2))
badI = -1
print('        %-15s    %s'% (argv[1], argv[2]))
for i in range(n):
	n1, i1, a1, c1, d1, l1 = matches1[i]
	n2, i2, a2, c2, d2, l2 = matches2[i]
	bad = i1 != i2 or a1 != a2 or c1 != c2 or d1 != d2
	marker = '******' if bad else ''
	print('%3d: (%6.2f %6.2f %3d)  (%6.2f %6.2f %3d) %s' % (i,
	     i1, a1, c1, i2, a2, c2, marker))
	if bad:
		print('%6d: %s' % (n1, l1))
		print('%6d: %s' % (n2, l2))
	if badI < 0:
		if bad:
			badI = i
	elif i > badI + 5:
		break

