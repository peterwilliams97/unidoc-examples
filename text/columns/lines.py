import re
from sys import argv

# leftMostWord: poolMinBaseIdx=188 startBaseIdx=188

regex = re.compile(r'leftMostWord:\s+poolMinBaseIdx=(\d+)\s+startBaseIdx=(\d+)')

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
			pool = int(m.group(1))
			start = int(m.group(2))
			# print('%d: %4d %4d' % (n, pool, start))
			matches.append((start, pool, line[:-1]))
	return nLines, matches

_, matches1 = scan(argv[1])
_, matches2 = scan(argv[2])
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))

n = min( len(matches1),  len(matches2))
badI = -1
print('        %-9s    %s'% (argv[1], argv[2]))
for i in range(n):
	p1, s1, l1 = matches1[i]
	p2, s2, l2 = matches2[i]
	marker = '******' if p1 != p2 or s1 != s2 else ''
	print('%3d: (%4d %4d)  (%4d %4d) %s | %s %s' % (i, p1, s1, p2, s2, l1[22:], l2[10:], marker))
	if badI < 0:
		if p1 != p2 or s1 != s2:
			badI = i
	elif i > badI + 5:
		break

