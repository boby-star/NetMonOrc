import unittest
from experiment_report import report


class ReportTests(unittest.TestCase):
    def data(self):
        return {'metadata': {'source':'live-compose-fault-injection', 'git_head':'test',
                'git_dirty':False, 'repetitions':2, 'modes':['adaptive'], 'scenarios':['http']},
                'trials':[{'repetition':1,'mode':'adaptive','scenario':'http','status':'completed',
                           'state_correct':True,'diagnosis_correct':False,'duration_ms':20,'completed_probes':2},
                          {'repetition':2,'mode':'adaptive','scenario':'http','status':'harness_error'}]}

    def test_missing_measurement_is_not_zero(self):
        text = report(self.data())
        self.assertIn('| adaptive | http | 2 | 1 | 0 | 0 | 1 | 1 |',text)
        self.assertIn('20 [20; 20] (n=1)',text)

    def test_duplicate_trial_rejected(self):
        data=self.data();data['trials'].append(data['trials'][0])
        with self.assertRaises(ValueError):report(data)


if __name__=='__main__':unittest.main()
