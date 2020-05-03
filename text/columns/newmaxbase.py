import re
from sys import argv

###1 found=true cnt=1 newMaxBase=180.85->180.85

regex= re.compile(r'###1 found=.+cnt=(\d+)\s+newMaxBase=(.+)->(.+)')
line =             '###1 found=true cnt=1 newMaxBase=180.85->180.85'
m = regex.search(line)
assert m


def scan(path):
	n = 0
	nLines = 0
	matches = []
	with open(path, 'rt', errors='ignore') as f:
		for line in f:
			nLines += 1
			m = regex.search(line)
			if not m:
				continue
			n += 1
			cnt = int(m.group(1))
			minBase = float(m.group(2))
			maxBase = float(m.group(3))
			matches.append((nLines, minBase, maxBase, cnt, line[:-1]))
	return nLines, matches

_, matches1 = scan(argv[1])
_, matches2 = scan(argv[2])
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))

n = min( len(matches1),  len(matches2))
badI = -1
print('        %-15s    %s'% (argv[1], argv[2]))
for i in range(n):
	n1, i1, a1, c1, l1 = matches1[i]
	n2, i2, a2, c2, l2 = matches2[i]
	bad = i1 != i2 or a1 != a2 or c1 !=c2
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

