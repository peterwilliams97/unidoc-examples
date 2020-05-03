import re
from sys import argv

# @@doShowText: "PERS"
reText= re.compile(r'@@doShowText: "(.*)"\s*$')
lineText = '@@doShowText: "PERS"'
assert reText.search(lineText)


def scan(path):
	nLines = 0
	matches = []
	with open(path, 'rt', errors='ignore') as f:
		for line in f:
			line = line[:-1]
			nLines += 1
			m = reText.search(line)
			if not m:
				continue
			text = m.group(1)
			matches.append((nLines, text, line))
	return nLines, matches

_, matches1 = scan(argv[1])
_, matches2 = scan(argv[2])
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))

n = min( len(matches1),  len(matches2))
badI = -1
print('        %-30s    %s'% (argv[1], argv[2]))
badIds = []
caseBadIds = set()
map1 = {}
map2 = {}

for i in range(n):
	n1, t1, l1 = matches1[i]
	n2, t2, l2 = matches2[i]
	bad = t1 != t2
	caseBad = t1.lower() != t2.lower()
	if bad:
		badIds.append(i)
	if caseBad:
		caseBadIds.add(i)
	part1 = '"%s"' % (t1)
	part2 = '"%s"' % (t2)
	marker = '******' if bad else ''
	# print('%3d: %-30s %-30s %s' % (i, part1, part2, marker))
	map1[i] = part1
	map2[i] = part2

print('-' * 80)
print('%d bad of %d' % (len(badIds), n))
print('%s %d matches' % (argv[1], len(matches1)))
print('%s %d matches' % (argv[2], len(matches2)))
print('bad=%d %s' % (len(badIds), badIds))

for i in badIds:
	marker = '******' if i in caseBadIds else ''
	print('%4d: %-25s %-25s %s' % (i, map1[i], map2[i], marker))
